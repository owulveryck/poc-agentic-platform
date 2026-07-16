package ppg.test

import rego.v1

violation contains v if {
	input.view == "a"
	v := {"id": "rule-a"}
}

violation contains v if {
	input.view == "b"
	v := {"id": "rule-b"}
}
