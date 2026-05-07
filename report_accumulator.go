package theater

type reportAccumulator struct {
	report            Report
	scenarioStatuses  map[string]Status
	logActCounts      map[string]int
	logDroppedRecords int
}

func newReportAccumulator() *reportAccumulator {
	return &reportAccumulator{
		scenarioStatuses: make(map[string]Status),
		logActCounts:     make(map[string]int),
	}
}

func (a *reportAccumulator) Apply(event Event) error {
	if err := event.Validate(); err != nil {
		return err
	}

	a.applyValidated(event)
	return nil
}

func (a *reportAccumulator) Report() (Report, error) {
	report := a.report

	for _, status := range a.scenarioStatuses {
		report.Summary.TotalScenarios++

		switch status {
		case StatusPassed:
			report.Summary.PassedScenarios++
		case StatusFailed:
			report.Summary.FailedScenarios++
		case StatusCanceled:
			report.Summary.CanceledScenarios++
		case StatusSkipped:
			report.Summary.SkippedScenarios++
		}
	}

	if report.Status == "" {
		report.Status = deriveStageStatus(report.Summary)
	}

	sortNodeReports(report.Nodes)
	sortLogRecords(report.Logs)
	report.LogSummary = a.logSummary()
	report.Failures = buildFailureIndex(report)

	if err := report.Validate(); err != nil {
		return Report{}, err
	}

	return report, nil
}

func (a *reportAccumulator) applyValidated(event Event) {
	if event.Generation != nil && a.report.Generation == nil {
		generation := *event.Generation
		a.report.Generation = &generation
	}

	if event.StageID != "" && a.report.StageID == "" {
		a.report.StageID = event.StageID
	}

	if event.StagePath != "" && a.report.StagePath == "" {
		a.report.StagePath = event.StagePath
	}

	if event.Kind == EventKindScenarioFinished && event.ScenarioPath != "" && event.Status.IsTerminal() {
		a.scenarioStatuses[event.ScenarioPath] = event.Status
	}

	if node, ok := nodeReportFromEvent(event); ok {
		a.report.Nodes = append(a.report.Nodes, node)
	}

	if event.Log != nil {
		a.applyLog(*event.Log)
	}

	if event.StagePath != "" && event.ScenarioPath == "" && event.Status.IsTerminal() {
		a.report.StageID = event.StageID
		a.report.StagePath = event.StagePath
		a.report.Status = event.Status
		a.report.Failure = event.Failure
		a.report.StartedAt = event.StartedAt
		a.report.EndedAt = event.EndedAt
		a.report.DurationMs = event.DurationMs
	}
}

func (a *reportAccumulator) applyLog(record LogRecord) {
	if record.Dropped {
		a.logDroppedRecords++
		return
	}

	key := logActCountKey(record)
	if len(a.report.Logs) >= DefaultScenarioLogRecordsPerRun ||
		a.logActCounts[key] >= DefaultScenarioLogRecordsPerAct {
		a.logDroppedRecords++
		return
	}

	cloned := cloneLogRecord(record)
	limitLogRecordForReport(&cloned)
	a.report.Logs = append(a.report.Logs, cloned)
	a.logActCounts[key]++
}

func (a *reportAccumulator) logSummary() *LogSummary {
	if len(a.report.Logs) == 0 && a.logDroppedRecords == 0 {
		return nil
	}

	summary := &LogSummary{
		Records:           len(a.report.Logs),
		DroppedRecords:    a.logDroppedRecords,
		PreviewLimitBytes: DefaultScenarioLogPreviewLimitBytes,
		PerActLimit:       DefaultScenarioLogRecordsPerAct,
		PerRunLimit:       DefaultScenarioLogRecordsPerRun,
	}
	for i := range a.report.Logs {
		if a.report.Logs[i].Truncated {
			summary.TruncatedRecords++
		}
	}

	return summary
}

func logActCountKey(record LogRecord) string {
	return record.ScenarioPath + "\x00" + record.ActID
}

func limitLogRecordForReport(record *LogRecord) {
	if record.Preview != nil && record.Preview.JSONValue != nil {
		record.Preview.JSONValue = nil
		record.Preview.Truncated = true
		record.Truncated = true
	}

	if record.Preview != nil && len(record.Preview.Text) > DefaultScenarioLogPreviewLimitBytes {
		text, _ := truncatePreviewMiddle(record.Preview.Text, DefaultScenarioLogPreviewLimitBytes)
		record.Preview.Text = text
		record.Preview.Truncated = true
		record.Truncated = true
	}

	if record.Payload != nil && record.Preview != nil && record.Preview.Truncated {
		record.Payload.Truncated = true
	}
}
