// Package resource interface with the resource abstractor
package resource

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"scheduler/calculate/schedulers/interfaces"
	"scheduler/logger"
	"strings"
)

var (
	RESOURCE_ABSTRACTOR_URL  = os.Getenv("RESOURCE_ABSTRACTOR_URL")
	RESOURCE_ABSTRACTOR_PORT = os.Getenv("RESOURCE_ABSTRACTOR_PORT")
)

const (
	PROTOCOL  = "http"
	RESOURCES = "/api/v1/resources"
)

func formatRequestParameters(r map[string]string) string {
	if len(r) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("?")
	for k, v := range r {
		if v == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s=%s&", k, v))
	}
	return sb.String()[:sb.Len()-1]
}

func AvailableResources[T interfaces.ResourceList](data *[]T, requestParameters map[string]string) error {
	url := fmt.Sprintf("%s://%s:%s%s/%s", PROTOCOL, RESOURCE_ABSTRACTOR_URL, RESOURCE_ABSTRACTOR_PORT, RESOURCES, formatRequestParameters(requestParameters))
	logger.DebugLogger().Printf("Request URL: %v", url)

	resp, err := http.Get(url)
	if err != nil {
		logger.ErrorLogger().Println("Error fetching resources")
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.ErrorLogger().Println("Error closing body")
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorLogger().Println("Error reading body")
		return err
	}

	err = json.Unmarshal(body, data)
	if err != nil {
		logger.ErrorLogger().Println("Error unmarshalling body")
		return err
	}

	return nil
}
