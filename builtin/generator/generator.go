package generator

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	randv2 "math/rand/v2"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

// Stable refs for built-in generators.
const (
	SequenceRef  = "sequence"
	UUIDRef      = "uuid"
	TimestampRef = "timestamp"
	StringRef    = "string"
	DigitsRef    = "digits"
	EmailRef     = "email"
	PhoneRef     = "phone"
	SlugRef      = "slug"
)

const (
	defaultStringAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	defaultTimestampFmt   = "rfc3339"
	defaultUUIDVersion    = "v4"
	uuidVersionV7         = "v7"
	defaultEmailStem      = "user"
	defaultSlugPrefix     = "item"
	runTokenLength        = 6
)

func descriptors() map[string]theater.GeneratorDef {
	return map[string]theater.GeneratorDef{
		SequenceRef:  sequenceDef(),
		UUIDRef:      uuidDef(),
		TimestampRef: timestampDef(),
		StringRef:    stringDef(),
		DigitsRef:    digitsDef(),
		EmailRef:     emailDef(),
		PhoneRef:     phoneDef(),
		SlugRef:      slugDef(),
	}
}

func sequenceDef() theater.GeneratorDef {
	return theater.GeneratorDef{
		Contract: theater.GeneratorContract{
			Summary: "deterministic per-binding stage-run sequence number",
			Args: []theater.ArgSpec{
				numberArg("start", false),
				numberArg("step", false),
			},
			Produces: theater.ValueContract{Kind: theater.ValueKindNumber, Required: true},
		},
		Validate: validateSequenceArgs,
		Generate: generateSequence,
	}
}

func uuidDef() theater.GeneratorDef {
	return theater.GeneratorDef{
		Contract: theater.GeneratorContract{
			Summary: "deterministic UUID string",
			Args: []theater.ArgSpec{
				stringArg("version", false),
			},
			Produces: theater.ValueContract{Kind: theater.ValueKindString, Required: true},
		},
		Validate: validateUUIDArgs,
		Generate: generateUUID,
	}
}

func timestampDef() theater.GeneratorDef {
	return theater.GeneratorDef{
		Contract: theater.GeneratorContract{
			Summary: "run-base timestamp with optional offset",
			Args: []theater.ArgSpec{
				stringArg("format", false),
				stringArg("offset", false),
			},
			Produces: theater.ValueContract{Kind: theater.ValueKindString, Required: true},
		},
		Validate: validateTimestampArgs,
		Generate: generateTimestamp,
	}
}

func stringDef() theater.GeneratorDef {
	return theater.GeneratorDef{
		Contract: theater.GeneratorContract{
			Summary: "deterministic pseudo-random string",
			Args: []theater.ArgSpec{
				numberArg("length", true),
				stringArg("alphabet", false),
			},
			Produces: theater.ValueContract{Kind: theater.ValueKindString, Required: true},
		},
		Validate: validateStringArgs,
		Generate: generateString,
	}
}

func digitsDef() theater.GeneratorDef {
	return theater.GeneratorDef{
		Contract: theater.GeneratorContract{
			Summary: "deterministic pseudo-random digit string",
			Args: []theater.ArgSpec{
				numberArg("length", true),
			},
			Produces: theater.ValueContract{Kind: theater.ValueKindString, Required: true},
		},
		Validate: validateDigitsArgs,
		Generate: generateDigits,
	}
}

func emailDef() theater.GeneratorDef {
	return theater.GeneratorDef{
		Contract: theater.GeneratorContract{
			Summary: "unique-looking email per binding and scenario invocation",
			Args: []theater.ArgSpec{
				stringArg("prefix", false),
				stringArg("stem", false),
				stringArg("domain", true),
			},
			Produces: theater.ValueContract{Kind: theater.ValueKindString, Required: true},
		},
		Validate: validateEmailArgs,
		Generate: generateEmail,
	}
}

func phoneDef() theater.GeneratorDef {
	return theater.GeneratorDef{
		Contract: theater.GeneratorContract{
			Summary: "deterministic phone-like string with finite suffix space and optional shuffled suffix order",
			Args: []theater.ArgSpec{
				stringArg("prefix", true),
				numberArg("digits", true),
				boolArg("random", false),
			},
			Produces: theater.ValueContract{Kind: theater.ValueKindString, Required: true},
		},
		Validate: validatePhoneArgs,
		Generate: generatePhone,
	}
}

func slugDef() theater.GeneratorDef {
	return theater.GeneratorDef{
		Contract: theater.GeneratorContract{
			Summary: "slug with deterministic run token and sequence suffix",
			Args: []theater.ArgSpec{
				stringArg("prefix", true),
				numberArg("max_length", false),
			},
			Produces: theater.ValueContract{Kind: theater.ValueKindString, Required: true},
		},
		Validate: validateSlugArgs,
		Generate: generateSlug,
	}
}

func stringArg(name string, required bool) theater.ArgSpec {
	return theater.ArgSpec{
		Name:     name,
		Accepts:  theater.ValueContract{Kind: theater.ValueKindString},
		Required: required,
	}
}

func numberArg(name string, required bool) theater.ArgSpec {
	return theater.ArgSpec{
		Name:     name,
		Accepts:  theater.ValueContract{Kind: theater.ValueKindNumber},
		Required: required,
	}
}

func boolArg(name string, required bool) theater.ArgSpec {
	return theater.ArgSpec{
		Name:     name,
		Accepts:  theater.ValueContract{Kind: theater.ValueKindBool},
		Required: required,
	}
}

func validateSequenceArgs(args theater.Values) error {
	if _, ok := args["step"]; ok {
		step, err := intArg(theater.Args(args), "step", 1)
		if err != nil {
			return err
		}
		if step <= 0 {
			return errors.New("step must be positive")
		}
	}

	if _, ok := args["start"]; ok {
		if _, err := intArg(theater.Args(args), "start", 0); err != nil {
			return err
		}
	}

	return nil
}

func validateUUIDArgs(args theater.Values) error {
	version := stringArgValue(theater.Args(args), "version", defaultUUIDVersion)
	if version != defaultUUIDVersion && version != uuidVersionV7 {
		return fmt.Errorf("version %q is not supported", version)
	}

	return nil
}

func validateTimestampArgs(args theater.Values) error {
	if _, ok := args["offset"]; ok {
		offset := stringArgValue(theater.Args(args), "offset", "")
		if _, err := time.ParseDuration(offset); err != nil {
			return fmt.Errorf("offset %q is invalid: %w", offset, err)
		}
	}

	format := stringArgValue(theater.Args(args), "format", defaultTimestampFmt)
	if format != defaultTimestampFmt {
		return fmt.Errorf("format %q is not supported", format)
	}

	return nil
}

func validateStringArgs(args theater.Values) error {
	length, err := intArg(theater.Args(args), "length", 0)
	if err != nil {
		return err
	}
	if length <= 0 {
		return errors.New("length must be positive")
	}

	if _, ok := args["alphabet"]; ok {
		alphabet := stringArgValue(theater.Args(args), "alphabet", "")
		if alphabet == "" {
			return errors.New("alphabet must not be empty")
		}
	}

	return nil
}

func validateDigitsArgs(args theater.Values) error {
	length, err := intArg(theater.Args(args), "length", 0)
	if err != nil {
		return err
	}
	if length <= 0 {
		return errors.New("length must be positive")
	}

	return nil
}

func validateEmailArgs(args theater.Values) error {
	prefix := stringArgValue(theater.Args(args), "prefix", "")
	stem := stringArgValue(theater.Args(args), "stem", "")
	if prefix != "" && stem != "" {
		return errors.New("prefix and stem are mutually exclusive")
	}

	domain := stringArgValue(theater.Args(args), "domain", "")
	if err := validateDomain(domain); err != nil {
		return err
	}

	return nil
}

func validatePhoneArgs(args theater.Values) error {
	prefix := stringArgValue(theater.Args(args), "prefix", "")
	if prefix == "" {
		return errors.New("prefix is required")
	}
	if strings.ContainsAny(prefix, " \t\r\n") {
		return errors.New("prefix must not contain whitespace")
	}

	digits, err := intArg(theater.Args(args), "digits", 0)
	if err != nil {
		return err
	}
	if digits <= 0 {
		return errors.New("digits must be positive")
	}
	if _, err := boolArgValue(theater.Args(args), "random", false); err != nil {
		return err
	}

	return nil
}

func validateSlugArgs(args theater.Values) error {
	prefix := stringArgValue(theater.Args(args), "prefix", "")
	if normalizeSlug(prefix) == "" {
		return errors.New("prefix must contain at least one alphanumeric character")
	}

	if _, ok := args["max_length"]; ok {
		length, err := intArg(theater.Args(args), "max_length", 0)
		if err != nil {
			return err
		}
		if length <= 0 {
			return errors.New("max_length must be positive")
		}
	}

	return nil
}

func generateSequence(request theater.GeneratorRequest) (any, error) {
	start, err := intArg(request.Args, "start", 0)
	if err != nil {
		return nil, err
	}

	step, err := intArg(request.Args, "step", 1)
	if err != nil {
		return nil, err
	}

	return start + step*request.SequenceIndex(), nil
}

func generateUUID(request theater.GeneratorRequest) (any, error) {
	version := stringArgValue(request.Args, "version", defaultUUIDVersion)
	switch version {
	case defaultUUIDVersion:
		bytes := randomBytes(request, "uuid-v4")
		bytes[6] = (bytes[6] & 0x0f) | 0x40
		bytes[8] = (bytes[8] & 0x3f) | 0x80
		return formatUUID(bytes), nil
	case uuidVersionV7:
		bytes := randomBytes(request, "uuid-v7")
		millis := uint64(request.Generation.BaseTime.Add(time.Duration(request.SequenceIndex()) * time.Millisecond).UnixMilli())
		bytes[0] = byte(millis >> 40)
		bytes[1] = byte(millis >> 32)
		bytes[2] = byte(millis >> 24)
		bytes[3] = byte(millis >> 16)
		bytes[4] = byte(millis >> 8)
		bytes[5] = byte(millis)
		bytes[6] = (bytes[6] & 0x0f) | 0x70
		bytes[8] = (bytes[8] & 0x3f) | 0x80
		return formatUUID(bytes), nil
	default:
		return nil, fmt.Errorf("version %q is not supported", version)
	}
}

func generateTimestamp(request theater.GeneratorRequest) (any, error) {
	offsetText := stringArgValue(request.Args, "offset", "0s")
	offset, err := time.ParseDuration(offsetText)
	if err != nil {
		return nil, fmt.Errorf("offset %q is invalid: %w", offsetText, err)
	}

	format := stringArgValue(request.Args, "format", defaultTimestampFmt)
	timestamp := request.Generation.BaseTime.Add(offset).UTC()
	switch format {
	case defaultTimestampFmt:
		return timestamp.Format(time.RFC3339), nil
	default:
		return nil, fmt.Errorf("format %q is not supported", format)
	}
}

func generateString(request theater.GeneratorRequest) (any, error) {
	length, err := intArg(request.Args, "length", 0)
	if err != nil {
		return nil, err
	}
	alphabet := stringArgValue(request.Args, "alphabet", defaultStringAlphabet)
	return randomString(request.DeriveRand("string"), alphabet, length), nil
}

func generateDigits(request theater.GeneratorRequest) (any, error) {
	length, err := intArg(request.Args, "length", 0)
	if err != nil {
		return nil, err
	}
	return randomString(request.DeriveRand("digits"), "0123456789", length), nil
}

func generateEmail(request theater.GeneratorRequest) (any, error) {
	stem := stringArgValue(request.Args, "stem", "")
	if stem == "" {
		stem = stringArgValue(request.Args, "prefix", defaultEmailStem)
	}
	local := normalizeEmailLocal(stem)
	if local == "" {
		local = defaultEmailStem
	}

	ordinal := request.SequenceIndex() + 1
	domain := strings.ToLower(stringArgValue(request.Args, "domain", ""))
	return fmt.Sprintf("%s-%s-%d@%s", local, request.RunToken(runTokenLength), ordinal, domain), nil
}

func generatePhone(request theater.GeneratorRequest) (any, error) {
	prefix := stringArgValue(request.Args, "prefix", "")
	digits, err := intArg(request.Args, "digits", 0)
	if err != nil {
		return nil, err
	}
	random, err := boolArgValue(request.Args, "random", false)
	if err != nil {
		return nil, err
	}

	index := request.SequenceIndex()
	limit := int64Pow10(digits)
	if index >= limit {
		return nil, fmt.Errorf("phone space exhausted for prefix %q and digits=%d", prefix, digits)
	}
	if random {
		index = shufflePhoneIndex(request, limit, index)
	}

	return fmt.Sprintf("%s%0*d", prefix, digits, index), nil
}

func generateSlug(request theater.GeneratorRequest) (any, error) {
	prefix := normalizeSlug(stringArgValue(request.Args, "prefix", ""))
	ordinal := request.SequenceIndex() + 1
	suffix := fmt.Sprintf("%s-%d", request.RunToken(runTokenLength), ordinal)
	if prefix == "" {
		prefix = defaultSlugPrefix
	}

	length, ok, err := optionalIntArg(request.Args, "max_length")
	if err != nil {
		return nil, err
	}
	if ok {
		if len(suffix)+1 > length {
			return nil, fmt.Errorf("max_length %d is too small for slug suffix", length)
		}

		prefix = fitSlugPrefix(prefix, suffix, length)
	}

	return prefix + "-" + suffix, nil
}

func fitSlugPrefix(prefix, suffix string, maxLength int) string {
	maxPrefix := maxLength - len(suffix) - 1
	if len(prefix) <= maxPrefix {
		return prefix
	}

	prefix = prefix[:maxPrefix]
	prefix = strings.Trim(prefix, "-")
	if prefix == "" {
		return defaultSlugPrefix
	}

	return prefix
}

func validateDomain(domain string) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return errors.New("domain is required")
	}
	if strings.ContainsAny(domain, " \t\r\n@") {
		return fmt.Errorf("domain %q is invalid", domain)
	}

	return nil
}

func intArg(args theater.Args, key string, defaultValue int64) (int64, error) {
	value, ok := args[key]
	if !ok {
		return defaultValue, nil
	}

	number, err := runtimevalue.Wrap(value).Int(key)
	if err != nil {
		return 0, err
	}

	return int64(number), nil
}

func optionalIntArg(args theater.Args, key string) (value int, ok bool, err error) {
	raw, ok := args[key]
	if !ok {
		return 0, false, nil
	}

	number, err := runtimevalue.Wrap(raw).Int(key)
	if err != nil {
		return 0, false, err
	}

	return number, true, nil
}

func boolArgValue(args theater.Args, key string, defaultValue bool) (bool, error) {
	raw, ok := args[key]
	if !ok {
		return defaultValue, nil
	}

	typed, err := runtimevalue.Bool(raw, key)
	if err != nil {
		return false, err
	}

	return typed, nil
}

func stringArgValue(args theater.Args, key, defaultValue string) string {
	value, ok := args[key]
	if !ok {
		return defaultValue
	}

	text, err := runtimevalue.Wrap(value).String(key)
	if err != nil {
		return defaultValue
	}

	return text
}

func randomString(rng *randv2.Rand, alphabet string, length int64) string {
	if length <= 0 {
		return ""
	}

	var builder strings.Builder
	builder.Grow(int(length))
	for range int(length) {
		builder.WriteByte(alphabet[rng.IntN(len(alphabet))])
	}

	return builder.String()
}

func randomBytes(request theater.GeneratorRequest, purpose string) [16]byte {
	var raw [16]byte
	rng := request.DeriveRand(purpose)
	binary.BigEndian.PutUint64(raw[:8], rng.Uint64())
	binary.BigEndian.PutUint64(raw[8:], rng.Uint64())
	return raw
}

func formatUUID(raw [16]byte) string {
	text := hex.EncodeToString(raw[:])
	return text[0:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:32]
}

func normalizeEmailLocal(raw string) string {
	return normalizeSlug(strings.ReplaceAll(raw, "@", "-"))
}

func normalizeSlug(raw string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(raw) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case lastDash:
			continue
		default:
			builder.WriteByte('-')
			lastDash = true
		}
	}

	return strings.Trim(builder.String(), "-")
}

func int64Pow10(length int64) int64 {
	value := int64(1)
	for range int(length) {
		value *= 10
	}

	return value
}

func shufflePhoneIndex(request theater.GeneratorRequest, limit, index int64) int64 {
	rng := request.DeriveRand("phone-random")
	offset := int64(rng.Uint64() % uint64(limit))
	step := phoneShuffleStep(rng, limit)
	return (step*index + offset) % limit
}

func phoneShuffleStep(rng *randv2.Rand, limit int64) int64 {
	candidate := int64(rng.Uint64()%uint64(limit-1)) + 1
	for {
		if candidate != 1 && candidate%2 != 0 && candidate%5 != 0 {
			return candidate
		}

		candidate++
		if candidate >= limit {
			candidate = 1
		}
	}
}
