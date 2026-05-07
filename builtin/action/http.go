package action

import (
	"context"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin/internal/builtinhttp"
)

type httpAction struct{}

func (httpAction) Contract() theater.ActionContract {
	return theater.ActionContract{
		Inputs: map[string]theater.ValueContract{
			"method":  {Kind: theater.ValueKindString},
			"url":     {Kind: theater.ValueKindString, Required: true},
			"headers": {Kind: theater.ValueKindObject},
			"body":    {Kind: theater.ValueKindString},
			"form":    {Kind: theater.ValueKindObject, Description: builtinhttp.FormArgDescription},
			"json":    {Kind: theater.ValueKindAny, Description: builtinhttp.JSONArgDescription},
			"timeout": {Kind: theater.ValueKindString},
			"session": {
				Kind:        theater.ValueKindString,
				Description: builtinhttp.SessionArgDescription,
				Sensitivity: theater.SensitivitySecret,
				Capture:     theater.CaptureOmit,
			},
			"identity": {
				Kind:        theater.ValueKindString,
				Description: builtinhttp.IdentityArgDescription,
			},
			"auth": {
				Kind:        theater.ValueKindString,
				Description: builtinhttp.AuthArgDescription,
			},
		},
		Outputs: map[string]theater.ValueContract{
			"status_code": {Kind: theater.ValueKindNumber},
			"status":      {Kind: theater.ValueKindString},
			"headers":     {Kind: theater.ValueKindObject},
			"body":        {Kind: theater.ValueKindString},
		},
	}
}

func (httpAction) Run(ctx context.Context, request theater.ActionRequest) (theater.Outputs, error) {
	httpRequest, err := builtinhttp.RequestFromArgs(request.Args)
	if err != nil {
		return theater.Outputs{}, err
	}

	response, err := builtinhttp.Do(ctx, request.Resources, request.HTTP, httpRequest)
	if err != nil {
		return theater.Outputs{}, err
	}

	outputs := builtinhttp.Outputs(response)
	if request.HTTPCapture != nil {
		if err := builtinhttp.CaptureAuth(request.Resources, request.HTTP, *request.HTTPCapture, response); err != nil {
			return outputs, httpActionError{
				cause:          err,
				summary:        "auth capture failed",
				partialOutputs: outputs,
			}
		}
	}

	return outputs, nil
}

type httpActionError struct {
	cause          error
	summary        string
	partialOutputs theater.Outputs
}

func (e httpActionError) Error() string {
	if e.cause == nil {
		return e.summary
	}

	return e.cause.Error()
}

func (e httpActionError) Unwrap() error {
	return e.cause
}

func (e httpActionError) FailureSummary() string {
	return e.summary
}

func (e httpActionError) PartialOutputs() theater.Outputs {
	return e.partialOutputs
}
