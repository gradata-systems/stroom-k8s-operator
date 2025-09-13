#!/bin/bash

keystore_path='/data/keystore.p12'
echo "Creating keystore: $keystore_path"
openssl pkcs12 -in /opt/tls/tls.crt -inkey /opt/tls/tls.key -export -out "$keystore_path" -passout "pass:${STROOM_KEYSTORE_PASSWORD}"

truststore_path='/data/truststore.p12'
echo "Creating truststore: $truststore_path"
openssl pkcs12 -in /opt/tls/ca.crt -nokeys -export -out "$truststore_path" -passout "pass:${STROOM_KEYSTORE_PASSWORD}" -jdktrust anyExtendedKeyUsage