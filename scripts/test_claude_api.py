import json
import os
import sys
import urllib.error
import urllib.request


secret_path = "/run/secrets/anthropic_api_key"
try:
    with open(secret_path, encoding="utf-8") as secret_file:
        api_key = secret_file.read().strip()
except OSError as error:
    sys.exit(f"Cannot read Podman secret at {secret_path}: {error}")

if not api_key:
    sys.exit("The Podman secret is empty.")

payload = json.dumps(
    {
        "model": os.environ["CLAUDE_MODEL"],
        "max_tokens": 24,
        "messages": [{"role": "user", "content": "Reply with exactly: API test passed"}],
    }
).encode()
request = urllib.request.Request(
    "https://api.anthropic.com/v1/messages",
    data=payload,
    headers={
        "content-type": "application/json",
        "x-api-key": api_key,
        "anthropic-version": "2023-06-01",
    },
    method="POST",
)

try:
    with urllib.request.urlopen(request, timeout=30) as response:
        result = json.load(response)
except urllib.error.HTTPError as error:
    detail = error.read().decode("utf-8", errors="replace")
    sys.exit(f"Anthropic API returned HTTP {error.code}: {detail}")
except (urllib.error.URLError, TimeoutError) as error:
    sys.exit(f"Could not reach Anthropic API: {error}")

text = " ".join(block.get("text", "") for block in result.get("content", []))
print(f"API request succeeded ({result.get('model', 'model unknown')}): {text}")
