package main

import (
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/TomaszJanusz/kitbag/internal/e2e"
)

// TestMain lets scripts run the real kitbag main function as a separate process.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){"kitbag": e2e.Guarded(testKitbagMain)})
}

// TestScripts runs the end-to-end scripts in testdata/script.
func TestScripts(t *testing.T) {
	t.Parallel()

	params := e2e.Params("testdata/script")
	configureAPIScripts(&params)
	params.Cmds["adoptschema"] = validatePlanCommand(namedSchema(t, "adopt"))
	params.Cmds["configschema"] = validateConfigCommand(namedSchema(t, "config"))
	params.Cmds["doctorschema"] = validatePlanCommand(namedSchema(t, "doctor"))
	params.Cmds["historyschema"] = validatePlanCommand(namedSchema(t, "history"))
	params.Cmds["execstatus"] = execStatus
	params.Cmds["planschema"] = validatePlanCommand(planSchema(t))
	testscript.Run(t, params)
}
