package theater

import (
	"context"
	"errors"
	"fmt"
)

const (
	logEvaluationStatusEmitted logEvaluationStatus = "emitted"
	logEvaluationStatusDropped logEvaluationStatus = "dropped"
	logEvaluationStatusError   logEvaluationStatus = "error"
)

type logEvaluationStatus string

type logEvaluationRecord struct {
	ID         string
	Path       string
	Attempt    int
	Status     logEvaluationStatus
	Value      any
	Message    string
	Fields     Values
	Dropped    bool
	Failure    *Failure
	SourceSpan *SourceRef
}

func (e actExecution) runLogs(
	ctx context.Context,
	actScope *valueScope,
	actionOutputs Values,
	attempt int,
) ([]logEvaluationRecord, *actOutcome, error) {
	if len(e.act.Logs) == 0 {
		return nil, nil, nil
	}

	records := make([]logEvaluationRecord, 0, len(e.act.Logs))
	for i := range e.act.Logs {
		log := e.act.Logs[i]
		if !e.reserveLog(log, attempt) {
			record := e.droppedLogRecord(log, attempt)
			records = append(records, record)
			if err := e.recordLogEvaluation(log, record); err != nil {
				return records, nil, err
			}
			continue
		}

		record, outcome := e.runLog(ctx, log, actScope, actionOutputs, attempt)
		records = append(records, record)
		if err := e.recordLogEvaluation(log, record); err != nil {
			return records, nil, err
		}
		if outcome != nil {
			outcome.logs = records
			return records, outcome, nil
		}
	}

	return records, nil, nil
}

func (e actExecution) runLog(
	ctx context.Context,
	log logPlan,
	actScope *valueScope,
	actionOutputs Values,
	attempt int,
) (logEvaluationRecord, *actOutcome) {
	path := e.logRuntimePath(log)
	record := logEvaluationRecord{
		ID:         log.ID,
		Path:       path,
		Attempt:    attempt,
		SourceSpan: cloneSourceRef(log.SourceSpan),
	}

	value, message, fields, err := e.evaluateLog(ctx, log, actScope, actionOutputs)
	if err == nil {
		record.Status = logEvaluationStatusEmitted
		record.Value = value
		record.Message = message
		record.Fields = fields
		return record, nil
	}

	failure := logEvaluationFailure(path, err)
	record.Status = logEvaluationStatusError
	record.Failure = failure
	if outcome := terminalLogEvaluationOutcome(err, failure); outcome != nil {
		return record, outcome
	}
	if log.Required {
		return record, &actOutcome{status: StatusFailed, failure: failure}
	}

	return record, nil
}

func (e actExecution) evaluateLog(
	ctx context.Context,
	log logPlan,
	actScope *valueScope,
	actionOutputs Values,
) (value any, message string, fields Values, err error) {
	if logValueConfigured(log.Value) {
		value, err = e.evaluateLogValue(ctx, log.Value, actScope, actionOutputs)
		if err != nil {
			return nil, "", nil, err
		}

		return value, "", nil, nil
	}

	fields = make(Values, len(log.Fields))
	for _, key := range sortedMapKeys(log.Fields) {
		value, err := e.evaluateLogValue(ctx, log.Fields[key], actScope, actionOutputs)
		if err != nil {
			return nil, "", nil, fmt.Errorf("field %q: %w", key, err)
		}

		fields[key] = value
	}

	return nil, log.Message, fields, nil
}

func (e actExecution) evaluateLogValue(
	ctx context.Context,
	value logValuePlan,
	actScope *valueScope,
	actionOutputs Values,
) (any, error) {
	switch {
	case value.Field != "":
		return e.logFieldResolver(actionOutputs, actScope).
			resolveNamedValueContext(ctx, value.Field, value.selectorPlan, "log field %q is missing")
	case value.Ref != "":
		return e.logRefResolver(actScope).
			resolveNamedValueContext(ctx, value.Ref, value.selectorPlan, "log ref %q is missing")
	case len(value.Object) != 0:
		return e.evaluateLogObject(ctx, value.Object, actScope, actionOutputs)
	case len(value.List) != 0:
		return e.evaluateLogList(ctx, value.List, actScope, actionOutputs)
	default:
		return nil, errors.New("log value source is missing")
	}
}

func (e actExecution) evaluateLogObject(
	ctx context.Context,
	values map[string]logValuePlan,
	actScope *valueScope,
	actionOutputs Values,
) (Values, error) {
	object := make(Values, len(values))
	for _, key := range sortedMapKeys(values) {
		value, err := e.evaluateLogValue(ctx, values[key], actScope, actionOutputs)
		if err != nil {
			return nil, fmt.Errorf("object field %q: %w", key, err)
		}

		object[key] = value
	}

	return object, nil
}

func (e actExecution) evaluateLogList(
	ctx context.Context,
	values []logValuePlan,
	actScope *valueScope,
	actionOutputs Values,
) ([]any, error) {
	list := make([]any, 0, len(values))
	for i := range values {
		value, err := e.evaluateLogValue(ctx, values[i], actScope, actionOutputs)
		if err != nil {
			return nil, fmt.Errorf("list item %d: %w", i, err)
		}

		list = append(list, value)
	}

	return list, nil
}

func (e actExecution) reserveLog(log logPlan, attempt int) bool {
	return e.logLimiter.reserve(LogRecord{
		ID:           log.ID,
		Path:         e.logRuntimePath(log),
		ScenarioPath: e.identity.scenarioPath,
		ActID:        e.act.ID,
		Attempt:      attempt,
		ScenarioSeq:  e.identity.scenarioSeq,
		Status:       LogStatusEmitted,
		SourceSpan:   cloneSourceRef(log.SourceSpan),
		Address:      e.recorder.addresses.logAddress(e.act.ID, log.ID).finished(attempt, nil),
	})
}

func (e actExecution) droppedLogRecord(log logPlan, attempt int) logEvaluationRecord {
	return logEvaluationRecord{
		ID:         log.ID,
		Path:       e.logRuntimePath(log),
		Attempt:    attempt,
		Status:     logEvaluationStatusDropped,
		Dropped:    true,
		SourceSpan: cloneSourceRef(log.SourceSpan),
	}
}

func (e actExecution) recordLogEvaluation(log logPlan, record logEvaluationRecord) error {
	status := StatusPassed
	failure := (*Failure)(nil)
	if record.Status == logEvaluationStatusError {
		status = StatusFailed
		failure = cloneFailure(record.Failure)
	}

	return e.recorder.record(Event{
		Kind:           EventKindLogEmitted,
		StageID:        e.identity.stageID,
		StagePath:      e.identity.stagePath,
		ScenarioID:     e.identity.scenarioID,
		ScenarioCallID: e.identity.scenarioCallID,
		ScenarioPath:   e.identity.scenarioPath,
		Path:           record.Path,
		Address:        e.recorder.addresses.logAddress(e.act.ID, log.ID).finished(record.Attempt, record.Failure),
		Attempt:        record.Attempt,
		ScenarioSeq:    e.identity.scenarioSeq,
		Status:         status,
		Failure:        failure,
		SourceSpan:     cloneSourceRef(log.SourceSpan),
		Log:            e.reportLogRecord(log, record),
	})
}

func (e actExecution) reportLogRecord(log logPlan, record logEvaluationRecord) *LogRecord {
	if record.Dropped {
		return &LogRecord{
			ID:             record.ID,
			Path:           record.Path,
			StageID:        e.identity.stageID,
			ScenarioID:     e.identity.scenarioID,
			ScenarioCallID: e.identity.scenarioCallID,
			ScenarioPath:   e.identity.scenarioPath,
			ActID:          e.act.ID,
			Attempt:        record.Attempt,
			ScenarioSeq:    e.identity.scenarioSeq,
			Status:         LogStatusOmitted,
			Format:         string(log.Format),
			SourceSpan:     cloneSourceRef(record.SourceSpan),
			Address:        e.recorder.addresses.logAddress(e.act.ID, log.ID).finished(record.Attempt, nil),
			Preview: &Preview{
				Kind:          observedPreviewKindUnknown,
				OmittedReason: "log_limit",
			},
			Dropped: true,
		}
	}

	if record.Status == logEvaluationStatusError {
		return &LogRecord{
			ID:             record.ID,
			Path:           record.Path,
			StageID:        e.identity.stageID,
			ScenarioID:     e.identity.scenarioID,
			ScenarioCallID: e.identity.scenarioCallID,
			ScenarioPath:   e.identity.scenarioPath,
			ActID:          e.act.ID,
			Attempt:        record.Attempt,
			ScenarioSeq:    e.identity.scenarioSeq,
			Status:         LogStatusError,
			Format:         string(log.Format),
			SourceSpan:     cloneSourceRef(record.SourceSpan),
			Address:        e.recorder.addresses.logAddress(e.act.ID, log.ID).finished(record.Attempt, record.Failure),
			Failure:        cloneFailure(record.Failure),
		}
	}

	observed := observeLogValue("log."+log.ID, logReportValue(record), ValueContract{
		Capture:     log.Capture,
		Sensitivity: log.Sensitivity,
	})
	status := LogStatusEmitted
	if observed.Preview != nil && observed.Preview.OmittedReason != "" {
		status = LogStatusOmitted
	}

	logRecord := &LogRecord{
		ID:             record.ID,
		Path:           record.Path,
		StageID:        e.identity.stageID,
		ScenarioID:     e.identity.scenarioID,
		ScenarioCallID: e.identity.scenarioCallID,
		ScenarioPath:   e.identity.scenarioPath,
		ActID:          e.act.ID,
		Attempt:        record.Attempt,
		ScenarioSeq:    e.identity.scenarioSeq,
		Status:         status,
		Format:         string(log.Format),
		SourceSpan:     cloneSourceRef(record.SourceSpan),
		Address:        e.recorder.addresses.logAddress(e.act.ID, log.ID).finished(record.Attempt, nil),
		Preview:        clonePreview(observed.Preview),
		Payload:        clonePayloadMetadata(observed.Payload),
	}
	if observed.Preview != nil {
		logRecord.Truncated = observed.Preview.Truncated
	}
	if observed.Payload != nil && observed.Payload.Truncated {
		logRecord.Truncated = true
	}

	return logRecord
}

func (e actExecution) logFieldResolver(actionOutputs Values, actScope *valueScope) referenceResolver {
	return newReferenceResolver(mapValueLookup(actionOutputs)).
		withBindingSource(actScope).
		withDecorators(e.catalog).
		withGeneration(e.catalog, e.generation, e.identity).
		withMatchers(e.matchers)
}

func (e actExecution) logRefResolver(actScope *valueScope) referenceResolver {
	return newReferenceResolver(actScope).
		withDecorators(e.catalog).
		withGeneration(e.catalog, e.generation, e.identity).
		withMatchers(e.matchers)
}

func (e actExecution) logRuntimePath(log logPlan) string {
	if log.ID == "" {
		return e.actPath + "/log"
	}

	return e.recorder.addresses.logPath(e.act.ID, log.ID)
}

func logEvaluationFailure(path string, err error) *Failure {
	return &Failure{
		Kind:    FailureKindObservation,
		Phase:   PhaseRun,
		At:      path,
		Summary: "log evaluation failed",
		Cause:   err,
	}
}

func logReportValue(record logEvaluationRecord) any {
	if record.Message == "" {
		return record.Value
	}

	value := Values{
		"message": record.Message,
	}
	if len(record.Fields) != 0 {
		value["fields"] = record.Fields
	}

	return value
}

func terminalLogEvaluationOutcome(err error, failure *Failure) *actOutcome {
	switch {
	case errors.Is(err, context.Canceled):
		return &actOutcome{status: StatusCanceled}
	case errors.Is(err, context.DeadlineExceeded):
		failure.Kind = FailureKindTimeout
		failure.Summary = "log evaluation timed out"
		return &actOutcome{status: StatusFailed, failure: failure}
	default:
		return nil
	}
}
