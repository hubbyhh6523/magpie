package pii

// translated says magpie runs under Rosetta: an Intel build on Apple
// silicon. The detection (internal/rosetta) is gone, so it is always
// false. Tests replace it.
var translated = func() bool { return false }

// emulated says magpie runs under Windows on Arm's x64 emulation: an x64
// build on an Arm64 computer. The detection (internal/winarm) is gone, so
// it is always false. Tests replace it.
var emulated = func() bool { return false }
