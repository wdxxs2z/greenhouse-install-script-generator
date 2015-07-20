msiexec /norestart /i diego.msi
  ADMIN_USERNAME=[USERNAME]
  ADMIN_PASSWORD=[PASSWORD]
  CONSUL_IPS=consul1.foo.bar
  CF_ETCD_CLUSTER=http://etcd1.foo.bar:4001
  STACK=windows2012R2
  REDUNDANCY_ZONE=zone1
  LOGGREGATOR_SHARED_SECRET=secret123
  ETCD_CA_FILE=%cd%\ca.crt
  ETCD_CERT_FILE=%cd%\client.crt
  ETCD_KEY_FILE=%cd%\client.key
