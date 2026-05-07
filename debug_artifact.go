package theater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

const (
	debugArtifactKindPause   debugArtifactKind = "pause"
	debugArtifactKindResume  debugArtifactKind = "resume"
	debugArtifactKindSummary debugArtifactKind = "summary"
)

type debugArtifactKind string

type debugArtifactRecord struct {
	Seq     uint64                       `json:"seq"`
	Kind    debugArtifactKind            `json:"kind"`
	Pause   *debugArtifactPauseRecord    `json:"pause,omitempty"`
	Resume  *debugArtifactResumeRecord   `json:"resume,omitempty"`
	Summary *debugArtifactSessionSummary `json:"summary,omitempty"`
}

type debugArtifactPauseRecord struct {
	Reason     string                `json:"reason,omitempty"`
	Breakpoint string                `json:"breakpoint,omitempty"`
	Snapshot   debugArtifactSnapshot `json:"snapshot"`
}

type debugArtifactResumeRecord struct {
	PauseSeq uint64 `json:"pause_seq"`
	Command  string `json:"command"`
}

type debugArtifactSessionSummary struct {
	Records uint64 `json:"records"`
}

type debugArtifactSnapshot struct {
	Ref       debugBoundaryRef      `json:"ref"`
	Status    Status                `json:"status"`
	Failure   *debugArtifactFailure `json:"failure,omitempty"`
	Scope     debugSnapshotSection  `json:"scope"`
	Inputs    debugSnapshotSection  `json:"inputs"`
	Output    debugSnapshotSection  `json:"output"`
	State     debugStateSnapshot    `json:"state"`
	Recent    debugRecentSnapshot   `json:"recent"`
	Scheduler debugSchedulerSummary `json:"scheduler"`
}

type debugArtifactFailure struct {
	Kind    FailureKind `json:"kind"`
	Phase   Phase       `json:"phase"`
	At      string      `json:"at"`
	Summary string      `json:"summary"`
	Cause   string      `json:"cause,omitempty"`
}

type debugArtifactSink struct {
	mu      sync.Mutex
	file    *os.File
	encoder *json.Encoder
	nextSeq uint64
}

func openDebugArtifactSink(path string) (*debugArtifactSink, error) {
	file, err := openPrivateDebugFileForRewrite(path)
	if err != nil {
		return nil, err
	}

	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)

	return &debugArtifactSink{
		file:    file,
		encoder: encoder,
	}, nil
}

func writePrivateDebugFile(path string, data []byte) error {
	file, err := openPrivateDebugFileForRewrite(path)
	if err != nil {
		return err
	}

	_, writeErr := file.Write(data)
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

func openPrivateDebugFileForRewrite(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := hardenDebugArtifactDir(dir); err != nil {
		return nil, err
	}
	if err := validateDebugArtifactPath(path); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := validateOpenDebugArtifactFile(path, file); err != nil {
		closeErr := file.Close()
		return nil, errors.Join(err, closeErr)
	}
	if err := file.Chmod(0o600); err != nil {
		closeErr := file.Close()
		return nil, errors.Join(err, closeErr)
	}
	if err := file.Truncate(0); err != nil {
		closeErr := file.Close()
		return nil, errors.Join(err, closeErr)
	}
	if _, err := file.Seek(0, 0); err != nil {
		closeErr := file.Close()
		return nil, errors.Join(err, closeErr)
	}

	return file, nil
}

func hardenDebugArtifactDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("debug artifact directory %q is not a directory", dir)
	}
	if mode := info.Mode().Perm(); mode&0o022 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("debug artifact directory %q must not be group/world writable: %w", dir, err)
		}
	}

	return nil
}

func validateDebugArtifactPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("debug artifact path %q must not be a symlink", path)
	}

	return nil
}

func validateOpenDebugArtifactFile(path string, file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("debug artifact path %q must be a regular file", path)
	}
	if linkCount(info) > 1 {
		return fmt.Errorf("debug artifact path %q must not have multiple hard links", path)
	}

	return nil
}

func linkCount(info os.FileInfo) uint64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 1
	}

	return stat.Nlink
}

func (s *debugArtifactSink) Close() error {
	if s == nil || s.file == nil {
		return nil
	}

	return s.file.Close()
}

func (s *debugArtifactSink) WritePause(
	ctx context.Context,
	reason string,
	breakpoint string,
	state debugBoundaryState,
) (uint64, error) {
	return s.write(ctx, debugArtifactRecord{
		Kind: debugArtifactKindPause,
		Pause: &debugArtifactPauseRecord{
			Reason:     reason,
			Breakpoint: breakpoint,
			Snapshot:   debugArtifactSnapshotFromBoundaryState(state),
		},
	})
}

func (s *debugArtifactSink) WriteResume(ctx context.Context, pauseSeq uint64, command string) (uint64, error) {
	return s.write(ctx, debugArtifactRecord{
		Kind: debugArtifactKindResume,
		Resume: &debugArtifactResumeRecord{
			PauseSeq: pauseSeq,
			Command:  command,
		},
	})
}

func (s *debugArtifactSink) WriteSummary(ctx context.Context, summary debugArtifactSessionSummary) (uint64, error) {
	return s.write(ctx, debugArtifactRecord{
		Kind:    debugArtifactKindSummary,
		Summary: &summary,
	})
}

func (s *debugArtifactSink) RecordCount() uint64 {
	if s == nil {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.nextSeq
}

func (s *debugArtifactSink) write(ctx context.Context, record debugArtifactRecord) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextSeq++
	record.Seq = s.nextSeq
	if err := s.encoder.Encode(debugArtifactJSONObject(record)); err != nil {
		return 0, err
	}

	return record.Seq, nil
}

func debugArtifactSnapshotFromBoundaryState(state debugBoundaryState) debugArtifactSnapshot {
	return debugArtifactSnapshot{
		Ref:       state.Ref,
		Status:    state.Status,
		Failure:   debugArtifactFailureFromFailure(state.Failure),
		Scope:     state.Scope,
		Inputs:    state.Inputs,
		Output:    state.Output,
		State:     state.State,
		Recent:    state.Recent,
		Scheduler: state.Scheduler,
	}
}

func debugArtifactFailureFromFailure(failure *Failure) *debugArtifactFailure {
	if failure == nil {
		return nil
	}

	snapshot := &debugArtifactFailure{
		Kind:    failure.Kind,
		Phase:   failure.Phase,
		At:      failure.At,
		Summary: failure.Summary,
	}
	if failure.Cause != nil {
		snapshot.Cause = failure.Cause.Error()
	}

	return snapshot
}

func debugArtifactJSONObject(record debugArtifactRecord) map[string]any {
	object := map[string]any{
		"seq":  record.Seq,
		"kind": record.Kind,
	}
	if record.Pause != nil {
		object["pause"] = debugArtifactPauseJSONObject(*record.Pause)
	}
	if record.Resume != nil {
		object["resume"] = map[string]any{
			"pause_seq": record.Resume.PauseSeq,
			"command":   record.Resume.Command,
		}
	}
	if record.Summary != nil {
		object["summary"] = map[string]any{
			"records": record.Summary.Records,
		}
	}

	return object
}

func debugArtifactPauseJSONObject(record debugArtifactPauseRecord) map[string]any {
	object := map[string]any{
		"snapshot": debugArtifactSnapshotJSONObject(record.Snapshot),
	}
	if record.Reason != "" {
		object["reason"] = record.Reason
	}
	if record.Breakpoint != "" {
		object["breakpoint"] = record.Breakpoint
	}

	return object
}

func debugArtifactSnapshotJSONObject(snapshot debugArtifactSnapshot) map[string]any {
	object := map[string]any{
		"ref":       debugBoundaryRefJSONObject(snapshot.Ref),
		"status":    snapshot.Status,
		"scope":     debugSnapshotSectionJSONObject(snapshot.Scope),
		"inputs":    debugSnapshotSectionJSONObject(snapshot.Inputs),
		"output":    debugSnapshotSectionJSONObject(snapshot.Output),
		"state":     debugStateSnapshotJSONObject(snapshot.State),
		"recent":    debugRecentSnapshotJSONObject(snapshot.Recent),
		"scheduler": debugSchedulerSummaryJSONObject(snapshot.Scheduler),
	}
	if snapshot.Failure != nil {
		object["failure"] = debugArtifactFailureJSONObject(*snapshot.Failure)
	}

	return object
}

func debugBoundaryRefJSONObject(ref debugBoundaryRef) map[string]any {
	object := map[string]any{
		"stage_id":         ref.StageID,
		"stage_path":       ref.StagePath,
		"scenario_id":      ref.ScenarioID,
		"scenario_call_id": ref.ScenarioCallID,
		"scenario_path":    ref.ScenarioPath,
		"path":             ref.Path,
		"kind":             ref.Kind,
		"phase":            ref.Phase,
		"attempt":          ref.Attempt,
	}
	if ref.SourceSpan != nil {
		object["source"] = debugSourceRefJSONObject(ref.SourceSpan)
	}

	return object
}

func debugArtifactFailureJSONObject(failure debugArtifactFailure) map[string]any {
	object := map[string]any{
		"kind":    failure.Kind,
		"phase":   failure.Phase,
		"at":      failure.At,
		"summary": failure.Summary,
	}
	if failure.Cause != "" {
		object["cause"] = failure.Cause
	}

	return object
}

func debugSnapshotSectionJSONObject(section debugSnapshotSection) map[string]any {
	fields := make([]map[string]any, 0, len(section.Fields))
	for i := range section.Fields {
		fields = append(fields, debugSnapshotFieldJSONObject(section.Fields[i]))
	}

	return map[string]any{
		"fields":  fields,
		"omitted": section.Omitted,
	}
}

func debugSnapshotFieldJSONObject(field debugSnapshotField) map[string]any {
	object := map[string]any{
		"key":    field.Key,
		"origin": field.Origin,
		"value":  debugSafeValueJSONObject(field.Value),
	}
	if field.SourceSpan != nil {
		object["source"] = debugSourceRefJSONObject(field.SourceSpan)
	}

	return object
}

func debugSafeValueJSONObject(value debugSafeValue) map[string]any {
	object := map[string]any{
		"kind":           value.Kind,
		"text":           value.Text,
		"size_hint":      value.SizeHint,
		"content_type":   value.ContentType,
		"redacted":       value.Redacted,
		"truncated":      value.Truncated,
		"omitted_reason": value.OmittedReason,
		"omitted":        value.Omitted,
	}
	if len(value.Children) != 0 {
		children := make([]map[string]any, 0, len(value.Children))
		for i := range value.Children {
			children = append(children, debugSnapshotFieldJSONObject(value.Children[i]))
		}
		object["children"] = children
	}

	return object
}

func debugStateSnapshotJSONObject(snapshot debugStateSnapshot) map[string]any {
	accesses := make([]map[string]any, 0, len(snapshot.Accesses))
	for i := range snapshot.Accesses {
		accesses = append(accesses, debugStateAccessJSONObject(snapshot.Accesses[i]))
	}
	enrichments := make([]map[string]any, 0, len(snapshot.Enrichments))
	for i := range snapshot.Enrichments {
		enrichments = append(enrichments, debugStateEnrichmentJSONObject(snapshot.Enrichments[i]))
	}

	return map[string]any{
		"accesses":    accesses,
		"enrichments": enrichments,
		"omitted":     snapshot.Omitted,
	}
}

func debugStateAccessJSONObject(access debugStateAccess) map[string]any {
	return map[string]any{
		"seq":   access.Seq,
		"op":    access.Op,
		"key":   access.Key,
		"value": debugSafeValueJSONObject(access.Value),
		"err":   access.Err,
	}
}

func debugStateEnrichmentJSONObject(enrichment debugStateEnrichment) map[string]any {
	return map[string]any{
		"backend": enrichment.Backend,
		"fields":  debugSnapshotSectionJSONObject(enrichment.Fields),
		"err":     enrichment.Err,
	}
}

func debugRecentSnapshotJSONObject(snapshot debugRecentSnapshot) map[string]any {
	items := make([]map[string]any, 0, len(snapshot.Items))
	for i := range snapshot.Items {
		items = append(items, debugEventSummaryJSONObject(snapshot.Items[i]))
	}

	return map[string]any{
		"items":   items,
		"omitted": snapshot.Omitted,
	}
}

func debugEventSummaryJSONObject(summary debugEventSummary) map[string]any {
	return map[string]any{
		"seq":     summary.Seq,
		"kind":    summary.Kind,
		"path":    summary.Path,
		"attempt": summary.Attempt,
		"text":    summary.Text,
	}
}

func debugSchedulerSummaryJSONObject(summary debugSchedulerSummary) map[string]any {
	return map[string]any{
		"focused_lane": summary.FocusedLane,
		"active":       summary.Active,
		"ready":        summary.Ready,
		"blocked":      summary.Blocked,
		"ready_paths":  cloneDebugReadyPaths(summary.ReadyPaths),
	}
}
