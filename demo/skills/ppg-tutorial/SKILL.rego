package ppg.skills.ppg_tutorial

import rego.v1

# Companion policy of the ppg-tutorial skill (dual-representation artifact).
# The skill is a read-and-run demo: it drives the validation server with curl and never
# edits project files, so it imposes no additional plan requirements. The
# file exists so the skill passes the enterprise gate (POST /validate_skill)
# like any tier >= 1 capability, and as the anchor for future requirements.
