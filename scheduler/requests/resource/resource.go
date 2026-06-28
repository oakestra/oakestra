// Package resource interfaces with the resource abstractor service.
package resource

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"scheduler/calculate/schedulers/placement"
	"scheduler/logger"
	"strings"
	"time"
)

var (
	resourceAbstractorURL  = os.Getenv("RESOURCE_ABSTRACTOR_URL")
	resourceAbstractorPort = os.Getenv("RESOURCE_ABSTRACTOR_PORT")
	// resourceBaseURL is built once at startup; host and port are fixed after process start.
	resourceBaseURL = fmt.Sprintf("%s://%s:%s%s/", protocol, resourceAbstractorURL, resourceAbstractorPort, resourcesPath)
	resourceClient  = &http.Client{Timeout: 10 * time.Second}
)

const (
	protocol      = "http"
	resourcesPath = "/api/v1/resources"
)

func formatQuery(requestParameters map[string]string, interestedResources []string) string {
	requestParams := formatRequestParameters(requestParameters)
	resources := formatInterestedResources(interestedResources)

	var sb strings.Builder
	sb.WriteString("?active=true")
	if requestParams != "" {
		sb.WriteString("&")
		sb.WriteString(requestParams)
	}
	if resources != "" {
		sb.WriteString("&")
		sb.WriteString(resources)
	}
	return sb.String()
}

func formatRequestParameters(r map[string]string) string {
	if len(r) == 0 {
		return ""
	}
	var sb strings.Builder
	for k, v := range r {
		if v == "" {
			continue
		}
		fmt.Fprintf(&sb, "%s=%s&", k, v)
	}
	return strings.TrimSuffix(sb.String(), "&")
}

func formatInterestedResources(interestedResources []string) string {
	if len(interestedResources) == 0 {
		return ""
	}
	return strings.Join(interestedResources, ",")
}

func AvailableResources[T placement.Candidate](data *[]T, requestParameters map[string]string, interestedResources []string) error {
	url := resourceBaseURL + formatQuery(requestParameters, interestedResources)
	logger.DebugLogger().Printf("Request URL: %v", url)

	resp, err := resourceClient.Get(url)
	if err != nil {
		logger.ErrorLogger().Println("Error fetching resources")
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.ErrorLogger().Println("Error closing body")
		}
	}()

	if err := json.NewDecoder(resp.Body).Decode(data); err != nil {
		logger.ErrorLogger().Println("Error decoding body")
		return err
	}

	return nil
}
