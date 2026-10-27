package text

func hostsText() []entry {
	return []entry{
		{WizardStepHosts, "Machines"},
		{WizardExplainHosts, "Which machines use this instance. Other machines ignore it and never warn about its directory."},
		{WizardHostsKeep, "Keep the current machines (%s)"},
		{WizardHostsHere, "This machine only (%s)"},
		{WizardHostsAll, "All machines"},
		{WizardHostsPick, "Choose machines…"},
		{WizardHostsPickHint, "Host names separated by commas, for example mac, runner02."},
		{WizardHostsKnown, "Machines named in kitbag.toml: %s"},
		{AgentInvalidHosts, "Enter at least one host name."},
		{AgentInvalidHost, "%s is not a valid host name: use letters, digits, dots, underscores or hyphens."},
		{AgentInvalidHostTwice, "%s is listed twice."},
		{WizardPreviewHosts, "Used only on %s; other machines ignore this instance."},
		{WizardPreviewAllHosts, "Used on every machine again."},
		{WizardPreviewEveryHost, "Used on every machine; machines without its directory will warn and skip it."},
		{AgentWarnUnknownHost, "%s is neither this machine nor named in kitbag.toml; check its spelling."},
	}
}
