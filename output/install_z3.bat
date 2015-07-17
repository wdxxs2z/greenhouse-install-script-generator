msiexec /norestart /i diego.msi
  ADMIN_USERNAME=[USERNAME]
  ADMIN_PASSWORD=[PASSWORD]
  CONSUL_IPS=10.244.0.54
  CF_ETCD_CLUSTER=http://10.244.0.42:4001
  STACK=windows2012R2
  REDUNDANCY_ZONE=z3
  LOGGREGATOR_SHARED_SECRET=loggregator-secret
  ETCD_CA_FILE=%cd%\ca.crt
  ETCD_CERT_FILE=%cd%\client.crt
  ETCD_KEY_FILE=%cd%\client.key
