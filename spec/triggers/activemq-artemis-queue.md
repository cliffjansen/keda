# ActiveMQ Artemis Queue Trigger

This specification describes the `activemq-artemis` trigger for ActiveMQ Artemis.

```yaml
  triggers:
  - type: activemq-artemis
    metadata:
      jolokiaHost: http://admin:adminpw@host:8161
      queueName: myqueue
      queueLength: '5' # Optional. Queue length target for HPA. Default: 5 messages
      passwordKey: JOLOKIA_PASSWORD
      trustCA: |
        -----BEGIN CERTIFICATE-----
        MIID/zCCAuegAwIBAgIUCgT1+4nERr1w5iIO6ZMEUFBGFLkwDQYJKoZIhvcNAQEL
	[ ... ]
        -----END CERTIFICATE-----  		  
```

The `jolokiaHost` value is the jolokia endpoint on the Artemis broker.

The `passwordKey` value is the name of the environment variable on your deployment containing the Jolokia password string. If supplied, the string value becomes or replaces the password in the jolokiaHost URL.  This is usually resolved from a `Secret V1` or a `ConfigMap V1` collections. `env` and `envFrom` are both supported.

## Example

[`examples/activemqartemis_scaledobject.yaml`](./../../examples/activemqartemis_scaledobject.yaml)