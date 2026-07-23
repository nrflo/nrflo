package tools_builtin

// fakeStepSession is a canned apirun.StepSession (Rule 4 forbids a real
// spawner/backend in these unit tests): it records every boundary stamp and
// rotation request and returns scripted RotateSignals values.
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
