Import("env")
import os

env_file = os.path.join(env.get("PROJECT_DIR"), ".env")
if os.path.exists(env_file):
    with open(env_file) as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                key, val = line.split("=", 1)
                key, val = key.strip(), val.strip()
                if val:  # skip empty values
                    if val.isdigit():
                        env.Append(CPPDEFINES=[(key, val)])
                    else:
                        env.Append(CPPDEFINES=[(key, env.StringifyMacro(val))])
