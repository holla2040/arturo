"""
Pre-build script: create dummy certificate .S files required by ESP-IDF components
(HTTPS server, ESP Rainmaker) when custom_sdkconfig triggers a source rebuild.

These components embed TLS certificates as assembly files. Since Arturo doesn't use
HTTPS server or Rainmaker, dummy files satisfy the build without real certificates.
"""
import os
Import("env")

CERTS = [
    "https_server.crt.S",
    "rmaker_mqtt_server.crt.S",
    "rmaker_claim_service_server.crt.S",
    "rmaker_ota_server.crt.S",
]

def create_dummy_certs(source, target, env):
    build_dir = env.subst("$BUILD_DIR")
    for cert in CERTS:
        path = os.path.join(build_dir, cert)
        if not os.path.exists(path):
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path, "w") as f:
                f.write("/* dummy cert - not used by Arturo */\n")
            print(f"  Created dummy cert: {cert}")

# Run before build starts
create_dummy_certs(None, None, env)
