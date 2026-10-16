package main

// builtFIDO2 and builtPIV are set from the compile tags. The window reads
// these so a -tags fido2 binary does not claim the hardware path is missing.
var (
	builtFIDO2 bool
	builtPIV   bool
)
