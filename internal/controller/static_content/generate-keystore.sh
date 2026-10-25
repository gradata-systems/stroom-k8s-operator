#!/bin/bash
# Creates the key store and trust store Stroom's HTTPS connector reads, from the node's TLS Secret
set -euo pipefail

tls_dir="${STROOM_TLS_DIR:-/opt/tls}"
keystore_dir="${STROOM_KEYSTORE_DIR:-/data}"

keystore_path="$keystore_dir/keystore.p12"
echo "Creating keystore: $keystore_path"
# The password is read from the environment, rather than passed as an argument, which other processes can see
openssl pkcs12 -in "$tls_dir/tls.crt" -inkey "$tls_dir/tls.key" -export -out "$keystore_path" -passout env:STROOM_KEYSTORE_PASSWORD

truststore_path="$keystore_dir/truststore.p12"
echo "Creating truststore: $truststore_path"
openssl pkcs12 -in "$tls_dir/ca.crt" -nokeys -export -out "$truststore_path" -passout env:STROOM_KEYSTORE_PASSWORD -jdktrust anyExtendedKeyUsage

# openssl creates the files readable only by their owner. The Stroom image starts as root, then runs Stroom as the
# stroom user, so when this runs as root, give the files to that user. Otherwise, the pod runs as a single user,
# who already owns them.
if [ "$(id -u)" = '0' ]; then
  chown stroom:stroom "$keystore_path" "$truststore_path"
fi
chmod 0400 "$keystore_path" "$truststore_path"
