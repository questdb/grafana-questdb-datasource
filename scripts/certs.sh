# creates crt and key pair that only work with 'require' tlsMode

openssl req -subj "/CN=qdb" -new \
-newkey rsa:2048 -days 3650 -nodes -x509 \
-keyout $PWD/config/keys/server.key \
-out $PWD/config/keys/server.crt

