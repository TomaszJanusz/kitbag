// Package model holds the shared view models rendered by the terminal UI and
// the settings window. Models are plain state machines: Update consumes a Msg
// (a user intent or a finished command) and returns Cmds for the renderer's
// runtime to execute with Runner. Models start no goroutines, perform no I/O,
// import no UI toolkit and keep every user-visible string in package text.
// They are not safe for concurrent use; one renderer loop owns each model.
package model
