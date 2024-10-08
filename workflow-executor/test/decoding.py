import json


decoder = json.JSONDecoder()
msg = '{"success": true, "message": "Function scheduled"}{"success": true, "func-id": 1, "operation": "invocation"}'

pos = 0
while pos < len(msg):
    obj, pos = decoder.raw_decode(msg, pos)
    print(obj, pos)
