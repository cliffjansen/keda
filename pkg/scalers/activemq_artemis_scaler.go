package scalers

import (
	"context"
	"fmt"
	"strconv"
	"io/ioutil"
	"net/http"
	"net/url"
	"encoding/json"
	"crypto/tls"
	"crypto/x509"
	"strings"

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
	trustCA            string
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
		meta.jolokiaHost = strings.TrimRight(val, "/")
	} else {
		return nil, fmt.Errorf("no jolokiaHost given")
	}
	u, err := url.Parse(meta.jolokiaHost)
	if err != nil {
		return nil, err
	}

	if val, ok := metadata["trustCA"]; ok && val != "" {
		meta.trustCA = val
	}

	if val, ok := metadata["passwordKey"]; ok && val != "" {
		var jolokiaPW string
		// If set in the deployment, KEDA passes in the resolved value of the password
		if val2, ok := resolvedEnv[val]; ok && val2 != ""  {
			jolokiaPW = val2
		} else {
			return nil, fmt.Errorf("password not set for deployment environment key %s", val)
		}
		var username string
		if u.User != nil {
			username = u.User.Username()
		}
		if username != "" {
			meta.jolokiaHost = fmt.Sprintf("%s://%s:%s@%s:%s", u.Scheme, username, jolokiaPW, u.Hostname(), u.Port())
		} else {
			return nil, fmt.Errorf("no username given for password")
		}
	}

	return &meta, nil
}

func (s *artemisScaler) IsActive(ctx context.Context) (bool, error) {
	length, err := GetArtemisQueueLength(
		ctx,
		s.metadata.jolokiaHost,
		s.metadata.queueName,
		s.metadata.trustCA,
	)
	if err != nil {
		log.Errorf("Artemis IsActive error %s", err)
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
		s.metadata.trustCA,
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
	MsgCount map[string] struct {
		MessageCount int32
	} `json:"value"`
	Status   int32 `json:"status"`
	Error    string `json:"error"`
}

func GetArtemisQueueLength(ctx context.Context, jolokiaHost string, queueName string, trustCA string) (int32, error) {
	var info MsgCountInfo

	url := fmt.Sprintf(
			"%s/console/jolokia/read/org.apache.activemq.artemis:broker=\"*\",component=addresses,address=%%22%s%%22/MessageCount",
			jolokiaHost,
			queueName,
		)

	// TODO: cache the http Client
	var roots *x509.CertPool = nil
	client := &http.Client{}

	if len(trustCA) > 0 {
		roots = x509.NewCertPool()    // override container's root CAs
		ok := roots.AppendCertsFromPEM([]byte(trustCA))
		if !ok {
			err := fmt.Errorf("bad Root CA encoding")
			return -1, err
		}
		tp := &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs: roots,
			InsecureSkipVerify: false,
		}}
		client = &http.Client{Transport: tp}
	}

	r, err := client.Get(url)
	if err != nil {
		return -1, err
	}
	defer r.Body.Close()
	if r.StatusCode == 403 {
		return -1, fmt.Errorf("Artemis query 403 permission error")
	}
	if r.StatusCode != 200 {
		return -1, fmt.Errorf("Artemis query failure, code %d", r.StatusCode)
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return -1, err
	}
	err = json.Unmarshal(b, &info)
	if err == nil {
		if info.Status == 200 {
			// Success!
			var total int32 = 0
			for _, v := range info.MsgCount {
				if v.MessageCount > 0 {
					total += v.MessageCount
				}
			}
			return total, nil
		}
		if info.Error != "" {
			return -1, fmt.Errorf("Artemis query error: %s", info.Error)
		}
		return -1, fmt.Errorf("Unknown Artemis error in broker query")
	}
	return -1, fmt.Errorf("Artemis query parsing error %s", err.Error())
}
