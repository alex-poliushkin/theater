package report

import "fmt"

// ObservedValue stores the report-safe observation of one action input or
// output.
type ObservedValue struct {
	Preview   *Preview         `json:"preview,omitempty"`
	Payload   *PayloadMetadata `json:"payload,omitempty"`
	Artifacts []ArtifactRef    `json:"artifacts,omitempty"`
}

// ObservedStream stores the report-safe observation of one streamed output.
type ObservedStream struct {
	Preview       *Preview         `json:"preview,omitempty"`
	Payload       *PayloadMetadata `json:"payload,omitempty"`
	DroppedChunks uint64           `json:"dropped_chunks,omitempty"`
}

// ActionObservations groups observed action inputs, outputs, and streams.
type ActionObservations struct {
	Inputs  map[string]ObservedValue  `json:"inputs,omitempty"`
	Outputs map[string]ObservedValue  `json:"outputs,omitempty"`
	Streams map[string]ObservedStream `json:"streams,omitempty"`
}

func (o ObservedValue) Validate() error {
	if o.Payload != nil {
		if err := o.Payload.Validate(); err != nil {
			return err
		}
	}

	return validateNodeArtifacts(o.Artifacts)
}

func (o ActionObservations) Validate() error {
	for key, value := range o.Inputs {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("input %q is invalid: %w", key, err)
		}
	}

	for key, value := range o.Outputs {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("output %q is invalid: %w", key, err)
		}
	}

	for key, value := range o.Streams {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("stream %q is invalid: %w", key, err)
		}
	}

	return nil
}

func (o ObservedStream) Validate() error {
	if o.Payload != nil {
		if err := o.Payload.Validate(); err != nil {
			return err
		}
	}

	return nil
}
