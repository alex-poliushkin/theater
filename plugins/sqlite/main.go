package main

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/alex-poliushkin/theater/plugin/manifest"
	"github.com/alex-poliushkin/theater/plugin/protocol"
	"github.com/alex-poliushkin/theater/plugin/sdk"

	_ "modernc.org/sqlite"
)

//go:embed manifest.json
var manifestJSON []byte

type sessionConfig struct {
	Profiles map[string]profile `json:"profiles"`
}

type profile struct {
	DSN      string              `json:"dsn"`
	Fixtures map[string][]string `json:"fixtures,omitempty"`
}

type serverState struct {
	config sessionConfig
}

func main() {
	file, err := manifest.UnmarshalFile(manifestJSON)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	server := sdk.NewServer(file)
	state := &serverState{}
	server.SetInitializeHandler(func(_ context.Context, params protocol.InitializeParams) error {
		var cfg sessionConfig
		if len(params.SessionConfig) != 0 {
			raw, err := json.Marshal(params.SessionConfig)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return err
			}
		}
		state.config = cfg
		return nil
	})

	if err := server.RegisterInventory(sdk.InventoryHandler{
		Capability: mustCapability(file, "inventory.sqlite.query"),
		Validate:   state.validateQuery,
		Resolve:    state.resolveQuery,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := server.RegisterAction(sdk.ActionHandler{
		Capability: mustCapability(file, "action.sqlite.seed.reset"),
		Validate:   state.validateSeedReset,
		Invoke:     state.invokeSeedReset,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := server.RegisterStateBackend(sdk.StateBackendHandler{
		Capability: mustCapability(file, "state_backend.sqlite"),
		Prepare:    state.prepareStateBackend,
		Read:       state.readStateRecord,
		CAS:        state.compareAndSetStateRecord,
		Claim:      state.claimStatePoolItem,
		Renew:      state.renewStateClaim,
		Release:    state.releaseStateClaim,
		Consume:    state.consumeStateClaim,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := server.ServeStdio(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func mustCapability(file manifest.File, name string) manifest.Capability {
	for i := range file.Capabilities {
		if file.Capabilities[i].Name == name {
			return file.Capabilities[i]
		}
	}

	panic("capability not found: " + name)
}

func (s *serverState) validateQuery(_ context.Context, params protocol.ValidateParams) (protocol.ValidateResult, error) {
	diagnostics := make([]protocol.ValidationDiagnostic, 0)
	profileName, ok := stringProperty(params.Properties, "profile")
	if ok {
		if _, err := s.profile(profileName); err != nil {
			diagnostics = append(diagnostics, protocol.ValidationDiagnostic{
				Path:    "/profile",
				Message: err.Error(),
			})
		}
	}

	return protocol.ValidateResult{Diagnostics: diagnostics}, nil
}

func (s *serverState) validateSeedReset(_ context.Context, params protocol.ValidateParams) (protocol.ValidateResult, error) {
	diagnostics := make([]protocol.ValidationDiagnostic, 0)
	profileName, ok := stringProperty(params.Properties, "profile")
	if ok {
		profile, profileErr := s.profile(profileName)
		if profileErr != nil {
			diagnostics = append(diagnostics, protocol.ValidationDiagnostic{
				Path:    "/profile",
				Message: profileErr.Error(),
			})
		} else {
			diagnostics = append(diagnostics, validateFixtureDiagnostic(profile, profileName, params.Properties)...)
		}
	}

	return protocol.ValidateResult{Diagnostics: diagnostics}, nil
}

func (s *serverState) resolveQuery(
	ctx context.Context,
	_ sdk.Emitter,
	params protocol.InventoryResolveParams,
) (protocol.InventoryResolveResult, error) {
	profileName, err := requiredStringProperty(params.Properties, "profile")
	if err != nil {
		return protocol.InventoryResolveResult{}, err
	}
	sqlText, err := requiredStringProperty(params.Properties, "sql")
	if err != nil {
		return protocol.InventoryResolveResult{}, err
	}
	expectOne := boolPropertyDefault(params.Properties, "expect_one", true)

	profile, err := s.profile(profileName)
	if err != nil {
		return protocol.InventoryResolveResult{}, err
	}

	db, err := sql.Open("sqlite", profile.DSN)
	if err != nil {
		return protocol.InventoryResolveResult{}, err
	}
	defer db.Close()

	queryArgs := sortedParams(params.Properties["params"])
	rows, err := db.QueryContext(ctx, sqlText, queryArgs...)
	if err != nil {
		return protocol.InventoryResolveResult{}, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return protocol.InventoryResolveResult{}, err
	}

	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		values, err := scanRow(rows, columns)
		if err != nil {
			return protocol.InventoryResolveResult{}, err
		}
		resultRows = append(resultRows, values)
	}
	if err := rows.Err(); err != nil {
		return protocol.InventoryResolveResult{}, err
	}

	if expectOne {
		switch len(resultRows) {
		case 0:
			return protocol.InventoryResolveResult{}, errors.New("query returned no rows")
		case 1:
			return protocol.InventoryResolveResult{Value: resultRows[0]}, nil
		default:
			return protocol.InventoryResolveResult{}, fmt.Errorf("query returned %d rows, expected exactly one", len(resultRows))
		}
	}

	return protocol.InventoryResolveResult{Value: resultRows}, nil
}

func (s *serverState) invokeSeedReset(
	ctx context.Context,
	emitter sdk.Emitter,
	params protocol.ActionInvokeParams,
) (protocol.ActionInvokeResult, error) {
	profileName, err := requiredStringProperty(params.Properties, "profile")
	if err != nil {
		return protocol.ActionInvokeResult{}, err
	}
	fixtureName, err := requiredStringProperty(params.Properties, "fixture")
	if err != nil {
		return protocol.ActionInvokeResult{}, err
	}

	profile, err := s.profile(profileName)
	if err != nil {
		return protocol.ActionInvokeResult{}, err
	}
	statements, ok := profile.Fixtures[fixtureName]
	if !ok {
		return protocol.ActionInvokeResult{}, &sdk.ActionError{
			Code:    "fixture_not_found",
			Message: fmt.Sprintf("fixture %q is not defined for profile %q", fixtureName, profileName),
		}
	}

	db, err := sql.Open("sqlite", profile.DSN)
	if err != nil {
		return protocol.ActionInvokeResult{}, err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return protocol.ActionInvokeResult{}, err
	}

	totalRows := int64(0)
	for i := range statements {
		if err := emitter.Progress(protocol.ProgressParams{
			Phase:   "sqlite.seed.reset",
			Message: fmt.Sprintf("executing statement %d of %d", i+1, len(statements)),
		}); err != nil {
			return protocol.ActionInvokeResult{}, err
		}

		result, err := tx.ExecContext(ctx, statements[i])
		if err != nil {
			_ = tx.Rollback()
			return protocol.ActionInvokeResult{}, &sdk.ActionError{
				Code:           "seed_reset_failed",
				Message:        err.Error(),
				PartialOutputs: map[string]any{"rows_affected": totalRows},
			}
		}
		affected, err := result.RowsAffected()
		if err == nil {
			totalRows += affected
		}
	}
	if err := tx.Commit(); err != nil {
		return protocol.ActionInvokeResult{}, err
	}

	_ = emitter.Log("seed reset complete", map[string]string{
		"profile":  profileName,
		"fixture":  fixtureName,
		"finished": time.Now().UTC().Format(time.RFC3339),
	})

	return protocol.ActionInvokeResult{
		Outputs: map[string]any{
			"rows_affected": totalRows,
		},
	}, nil
}

func (s *serverState) profile(name string) (profile, error) {
	if s == nil {
		return profile{}, errors.New("server state is required")
	}
	if s.config.Profiles == nil {
		return profile{}, fmt.Errorf("profile %q is not configured", name)
	}

	profileConfig, ok := s.config.Profiles[name]
	if !ok {
		return profile{}, fmt.Errorf("profile %q is not configured", name)
	}
	if profileConfig.DSN == "" {
		return profile{}, fmt.Errorf("profile %q dsn is required", name)
	}

	return profileConfig, nil
}

func validateFixtureDiagnostic(profile profile, profileName string, properties map[string]any) []protocol.ValidationDiagnostic {
	fixtureName, ok := stringProperty(properties, "fixture")
	if !ok {
		return nil
	}
	if _, exists := profile.Fixtures[fixtureName]; exists {
		return nil
	}

	return []protocol.ValidationDiagnostic{{
		Path:    "/fixture",
		Message: fmt.Sprintf("fixture %q is not defined for profile %q", fixtureName, profileName),
	}}
}

func requiredStringProperty(properties map[string]any, key string) (string, error) {
	value, ok := stringProperty(properties, key)
	if !ok {
		return "", fmt.Errorf("property %q must be a string", key)
	}
	if value == "" {
		return "", fmt.Errorf("property %q is required", key)
	}
	return value, nil
}

func stringProperty(properties map[string]any, key string) (string, bool) {
	if properties == nil {
		return "", false
	}
	value, ok := properties[key].(string)
	return value, ok
}

func boolPropertyDefault(properties map[string]any, key string, fallback bool) bool {
	if properties == nil {
		return fallback
	}
	value, ok := properties[key].(bool)
	if !ok {
		return fallback
	}
	return value
}

func sortedParams(raw any) []any {
	params, ok := raw.(map[string]any)
	if !ok || len(params) == 0 {
		return nil
	}

	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	values := make([]any, 0, len(keys))
	for _, key := range keys {
		values = append(values, params[key])
	}
	return values
}

func scanRow(rows *sql.Rows, columns []string) (map[string]any, error) {
	values := make([]any, len(columns))
	dest := make([]any, len(columns))
	for i := range dest {
		dest[i] = &values[i]
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, err
	}

	result := make(map[string]any, len(columns))
	for i := range columns {
		switch value := values[i].(type) {
		case []byte:
			result[columns[i]] = string(value)
		default:
			result[columns[i]] = value
		}
	}

	return result, nil
}
