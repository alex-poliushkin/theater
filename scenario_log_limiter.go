package theater

import "sync"

type scenarioLogLimiter struct {
	mu        sync.Mutex
	actCounts map[string]int
	runCount  int
}

func newScenarioLogLimiter() *scenarioLogLimiter {
	return &scenarioLogLimiter{
		actCounts: make(map[string]int),
	}
}

func (l *scenarioLogLimiter) reserve(record LogRecord) bool {
	if l == nil {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	key := logActCountKey(record)
	if l.runCount >= DefaultScenarioLogRecordsPerRun ||
		l.actCounts[key] >= DefaultScenarioLogRecordsPerAct {
		return false
	}

	l.runCount++
	l.actCounts[key]++
	return true
}
