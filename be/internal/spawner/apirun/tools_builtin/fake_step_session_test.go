package tools_builtin

import "context"

// fakeStepSession is a canned apirun.StepSession (Rule 4 forbids a real
// spawner/backend in these unit tests): it records every boundary stamp and
// rotation request and returns scripted RotateSignals/RunStepChecks values.
type fakeStepSession struct {
	contextTokens   int
	thresholdTokens int

	rotateSignalsCalls int
	boundaryCalls      []string
	rotationRequests   []string
	// onRequestRotation, when set, runs synchronously inside
	// RequestStepRotation — used to assert DB ordering (the cursor is
	// already advanced by the time rotation is requested).
	onRequestRotation func()

	// checksFailedIdx/checksExitCode/checksOutputTail/checksErr script
	// RunStepChecks' return (default: no checks configured, all pass);
	// checksCmds records what it was handed.
	checksFailedIdx  int
	checksExitCode   int
	checksOutputTail string
	checksErr        error
	checksConfigured bool
	checksCmds       []string
}

func (f *fakeStepSession) RotateSignals(sessionID string) (int, int) {
	f.rotateSignalsCalls++
	return f.contextTokens, f.thresholdTokens
}

func (f *fakeStepSession) NoteStepBoundary(sessionID string) {
	f.boundaryCalls = append(f.boundaryCalls, sessionID)
}

func (f *fakeStepSession) RequestStepRotation(sessionID string) {
	f.rotationRequests = append(f.rotationRequests, sessionID)
	if f.onRequestRotation != nil {
		f.onRequestRotation()
	}
}

func (f *fakeStepSession) RunStepChecks(ctx context.Context, sessionID string, cmds []string) (failedIdx, exitCode int, outputTail string, err error) {
	f.checksCmds = cmds
	if !f.checksConfigured {
		return -1, 0, "", nil
	}
	return f.checksFailedIdx, f.checksExitCode, f.checksOutputTail, f.checksErr
}
