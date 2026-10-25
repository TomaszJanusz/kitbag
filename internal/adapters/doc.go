// Package adapters contains coding-agent implementations. The shared declaration
// contract lives in internal/target so core never imports adapter implementations.
// Put each agent's paths and settings logic in its own subpackage; instruction
// files are rendered only by the shared instructions subpackage. Keep
// synchronization policy in internal/core.
package adapters
