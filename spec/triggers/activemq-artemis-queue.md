# ActiveMQ Artemis Queue Trigger

This specification describes the `activemq-artemis` trigger for ActiveMQ Artemis.

```yaml
  triggers:
  - type: activemq-artemis
    metadata:
      jolokiaHost: http://admin:adminpw@host:8161
      queueName: myqueue
      queueLength: '5' # Optional. Queue length target for HPA. Default: 5 messages
```

The `jolokiaHost` value is the jolokia endpoint on the Artemis broker.

## Example

[`examples/activemqartemis_scaledobject.yaml`](./../../examples/activemqartemis_scaledobject.yaml)