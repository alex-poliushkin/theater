package decorator

import (
	"encoding/csv"
	"errors"
	"fmt"
	"strings"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

func csvDecoratorDef() theater.DecoratorDef {
	return theater.DecoratorDef{
		Contract: theater.DecoratorContract{
			Accepts:  theater.ValueContract{Kinds: theater.NewValueKindSet(theater.ValueKindString, theater.ValueKindBytes)},
			Produces: theater.ValueContract{Kind: theater.ValueKindList},
			Params: []theater.ParamSpec{
				{Name: "comma", Accepts: theater.ValueContract{Kind: theater.ValueKindString}, Default: ","},
				{Name: "comment", Accepts: theater.ValueContract{Kind: theater.ValueKindString}},
				{Name: "trim_leading_space", Accepts: theater.ValueContract{Kind: theater.ValueKindBool}, Default: false},
				{Name: "fields_per_record", Accepts: theater.ValueContract{Kind: theater.ValueKindNumber}, Default: -1},
			},
			Summary: "decode CSV text into a list of row objects keyed by header",
		},
		Compile: func(args theater.Values) (theater.DecoratorFunc, error) {
			settings, err := compileCSVSettings(args)
			if err != nil {
				return nil, err
			}

			return func(value any) (any, error) {
				text, err := csvTextValue(value)
				if err != nil {
					return nil, err
				}

				reader := csv.NewReader(strings.NewReader(text))
				reader.Comma = settings.Comma
				reader.Comment = settings.Comment
				reader.TrimLeadingSpace = settings.TrimLeadingSpace
				reader.FieldsPerRecord = settings.FieldsPerRecord

				records, err := reader.ReadAll()
				if err != nil {
					return nil, err
				}

				decoded, err := csvRecords(records)
				if err != nil {
					return nil, err
				}

				return runtimevalue.PreserveSecret(value, decoded), nil
			}, nil
		},
	}
}

func csvRecords(records [][]string) ([]any, error) {
	if len(records) == 0 {
		return []any{}, nil
	}

	headers := records[0]
	if err := validateCSVHeaders(headers); err != nil {
		return nil, err
	}

	rows := make([]any, 0, len(records)-1)

	for rowIndex, record := range records[1:] {
		if len(record) != len(headers) {
			return nil, fmt.Errorf("csv row %d field count mismatch: got %d want %d", rowIndex+2, len(record), len(headers))
		}

		row := make(map[string]any, len(headers))
		for i := range headers {
			row[headers[i]] = record[i]
		}

		rows = append(rows, row)
	}

	return rows, nil
}

func validateCSVHeaders(headers []string) error {
	seen := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		if _, ok := seen[header]; ok {
			return fmt.Errorf("csv header %q is duplicated", header)
		}

		seen[header] = struct{}{}
	}

	return nil
}

type csvSettings struct {
	Comma            rune
	Comment          rune
	TrimLeadingSpace bool
	FieldsPerRecord  int
}

func compileCSVSettings(args theater.Values) (csvSettings, error) {
	settings := csvSettings{
		Comma:           ',',
		FieldsPerRecord: -1,
	}

	if value, ok := args["comma"]; ok {
		r, err := singleRune(value, "comma")
		if err != nil {
			return csvSettings{}, err
		}
		settings.Comma = r
	}

	if value, ok := args["comment"]; ok {
		r, err := singleRune(value, "comment")
		if err != nil {
			return csvSettings{}, err
		}
		settings.Comment = r
	}

	if settings.Comment != 0 && settings.Comment == settings.Comma {
		return csvSettings{}, errors.New("comment must differ from comma")
	}

	if value, ok := args["trim_leading_space"]; ok {
		typed, err := runtimevalue.Bool(value, "trim_leading_space")
		if err != nil {
			return csvSettings{}, fmt.Errorf("trim_leading_space must be bool, got %T", value)
		}
		settings.TrimLeadingSpace = typed
	}

	if value, ok := args["fields_per_record"]; ok {
		typed, err := intValue(value, "fields_per_record")
		if err != nil {
			return csvSettings{}, err
		}
		settings.FieldsPerRecord = typed
	}

	return settings, nil
}

func csvTextValue(value any) (string, error) {
	wrapped := runtimevalue.Wrap(value)
	if bytes, ok := wrapped.BytesOK(); ok {
		return string(bytes), nil
	}

	if text, ok := wrapped.StringOK(); ok {
		return text, nil
	}

	return "", fmt.Errorf("csv decorator input must be string or []byte, got %T", value)
}
