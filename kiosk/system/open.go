package system

// Open returns the system the program runs on. It does not fail: a system
// gift does not know reports what little it can and refuses every change.
func Open() System { return open() }
