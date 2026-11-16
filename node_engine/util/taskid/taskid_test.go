package taskid

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestServiceExtractionFromTaskId(t *testing.T) {
	serviceName := "test.test.nginx.test"
	taskId := Generate(serviceName, 23)
	extracted := ExtractServiceName(taskId)
	assert.Equal(t, extracted, serviceName)
}

func TestServiceExtractionFromTaskIdMultipleInstance(t *testing.T) {
	serviceName := "test.test.instance.test"
	taskId := Generate(serviceName, 23)
	extracted := ExtractServiceName(taskId)
	assert.Equal(t, extracted, serviceName)
}

func TestInstanceExtractionFromTaskId(t *testing.T) {
	serviceName := "test.test.nginx.test"
	instanceId := 23
	taskId := Generate(serviceName, instanceId)
	extracted := ExtractInstanceNumber(taskId)
	assert.Equal(t, extracted, instanceId)
}

func TestInstanceExtractionFromTaskIdMultipleInstance(t *testing.T) {
	serviceName := "test.test.instance.test"
	instanceId := 23
	taskId := Generate(serviceName, instanceId)
	extracted := ExtractInstanceNumber(taskId)
	assert.Equal(t, extracted, instanceId)
}
