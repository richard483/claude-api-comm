#!/usr/bin/env bash
# Minimal fake of `claude -p ... --output-format stream-json`.
# Emits an init line, one text line, one tool_use line, and a result line.
echo '{"type":"system","subtype":"init","session_id":"fake-sess","tools":["Bash"]}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"working"}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}'
echo '{"type":"result","subtype":"success","result":"all done","session_id":"fake-sess","num_turns":1,"total_cost_usd":0.02,"duration_ms":100}'
exit 0
