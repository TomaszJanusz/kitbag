// Package adapters contains coding-agent implementations. The shared declaration
// contract lives in internal/target so core never imports adapter implementations.
// Put each agent's paths, instruction formats and settings logic in its own
// subpackage; keep synchronization policy in internal/core.
package adapters
