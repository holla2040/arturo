Import("env")
import os
import sys

REQUIRED_KEYS = ("WIFI_SSID", "WIFI_PASSWORD", "REDIS_HOST", "STATION_INSTANCE")

env_file = os.path.join(env.get("PROJECT_DIR"), ".env")
if not os.path.exists(env_file):
    sys.stderr.write(
        "\n*** load_env.py: .env not found at %s\n"
        "    Copy env-template to .env and fill in your values.\n\n" % env_file
    )
    env.Exit(1)

loaded = {}
with open(env_file) as f:
    for lineno, raw in enumerate(f, 1):
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, val = line.split("=", 1)
        key, val = key.strip(), val.strip()
        # Strip matched surrounding quotes; reject unmatched (common typo)
        if len(val) >= 2 and val[0] in ('"', "'") and val[-1] == val[0]:
            val = val[1:-1]
        elif val and (val[0] in ('"', "'") or val[-1] in ('"', "'")):
            sys.stderr.write(
                "\n*** load_env.py: %s line %d: unmatched quote in value for %s\n"
                "    Got: %s\n\n" % (env_file, lineno, key, val)
            )
            env.Exit(1)
        loaded[key] = val

# Process env wins over .env (lets the Makefile pass per-build values like STATION_INSTANCE)
for key in REQUIRED_KEYS:
    if os.environ.get(key):
        loaded[key] = os.environ[key]

for key, val in loaded.items():
    if not val:
        continue
    if val.isdigit():
        env.Append(CPPDEFINES=[(key, val)])
    else:
        env.Append(CPPDEFINES=[(key, env.StringifyMacro(val))])

missing = [k for k in REQUIRED_KEYS if not loaded.get(k)]
if missing:
    sys.stderr.write(
        "\n*** load_env.py: missing or empty in %s: %s\n"
        "    Compare against env-template — a misspelled key is silently ignored.\n\n"
        % (env_file, ", ".join(missing))
    )
    env.Exit(1)
