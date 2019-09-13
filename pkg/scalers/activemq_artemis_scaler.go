package scalers

import (
	"context"
	"fmt"
	"strconv"
	"io/ioutil"
	"net/http"
	"encoding/json"

	log "github.com/Sirupsen/logrus"
	v2beta1 "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

const (
	artemisMetricName               = "queueLength"
	artemisDefaultTargetQueueLength = 5
	artemisMetricType               = "External"
)

type artemisScaler struct {
	metadata *artemisMetadata
}

type artemisMetadata struct {
	targetQueueLength int
	queueName         string
	jolokiaHost       string
}

func NewActivemqArtemisScaler(resolvedEnv, metadata map[string]string) (Scaler, error) {
	meta, err := parseArtemisMetadata(metadata, resolvedEnv)
	if err != nil {
		return nil, fmt.Errorf("error parsing ActiveMQ Artemis queue metadata: %s", err)
	}

	return &artemisScaler{
		metadata: meta,
	}, nil
}

func parseArtemisMetadata(metadata, resolvedEnv map[string]string) (*artemisMetadata, error) {
	meta := artemisMetadata{}
	meta.targetQueueLength = artemisDefaultTargetQueueLength

	if val, ok := metadata[artemisMetricName]; ok {
		queueLength, err := strconv.Atoi(val)
		if err != nil {
			log.Errorf("Error parsing ActiveMQ Artemis metadata %s: %s", artemisMetricName, err.Error())
			return nil, fmt.Errorf("Error parsing ActiveMQ Artemis metadata %s: %s", artemisMetricName, err.Error())
		}

		meta.targetQueueLength = queueLength
	}

	if val, ok := metadata["queueName"]; ok && val != "" {
		meta.queueName = val
	} else {
		return nil, fmt.Errorf("no queueName given")
	}

	if val, ok := metadata["jolokiaHost"]; ok && val != "" {
		meta.jolokiaHost = val
	} else {
		return nil, fmt.Errorf("no jolokiaHost given")
	}

	return &meta, nil
}

func (s *artemisScaler) IsActive(ctx context.Context) (bool, error) {
	length, err := GetArtemisQueueLength(
		ctx,
		s.metadata.jolokiaHost,
		s.metadata.queueName,
	)

	if err != nil {
		log.Errorf("error %s", err)
		return false, err
	}

	return length > 0, nil
}

func (s *artemisScaler) Close() error {
	return nil
}

func (s *artemisScaler) GetMetricSpecForScaling() []v2beta1.MetricSpec {
	targetQueueLengthQty := resource.NewQuantity(int64(s.metadata.targetQueueLength), resource.DecimalSI)
	externalMetric := &v2beta1.ExternalMetricSource{MetricName: artemisMetricName, TargetAverageValue: targetQueueLengthQty}
	metricSpec := v2beta1.MetricSpec{External: externalMetric, Type: artemisMetricType}
	return []v2beta1.MetricSpec{metricSpec}
}

//GetMetrics returns value for a supported metric and an error if there is a problem getting the metric
func (s *artemisScaler) GetMetrics(ctx context.Context, metricName string, metricSelector labels.Selector) ([]external_metrics.ExternalMetricValue, error) {
	queuelen, err := GetArtemisQueueLength(
		ctx,
		s.metadata.jolokiaHost,
		s.metadata.queueName,
	)

	if err != nil {
		log.Errorf("error getting queue length %s", err)
		return []external_metrics.ExternalMetricValue{}, err
	}

	metric := external_metrics.ExternalMetricValue{
		MetricName: metricName,
		Value:      *resource.NewQuantity(int64(queuelen), resource.DecimalSI),
		Timestamp:  metav1.Now(),
	}

	return append([]external_metrics.ExternalMetricValue{}, metric), nil
}


type MsgCountInfo struct {
	MsgCount   int32 `json:"value"`
}

func GetArtemisQueueLength(ctx context.Context, jolokiaHost string, queueName string) (int32, error) {
	var info MsgCountInfo

	url := fmt.Sprintf(
			"%s/console/jolokia/read/org.apache.activemq.artemis:broker=\"0.0.0.0\",component=addresses,address=%%22%s%%22/MessageCount",
			jolokiaHost,
			queueName,
		)

	r, err := http.Get(url)
	if err != nil {
		return -1, err
	}
	defer r.Body.Close()
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return -1, err
	}
	err = json.Unmarshal(b, &info)
	if err != nil {
		return -1, err
	}

	return info.MsgCount, nil
}
