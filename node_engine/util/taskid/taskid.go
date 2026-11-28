package taskid

import (
	"fmt"
	"go_node_engine/model"
	"strconv"
	"strings"
)

const (
	separator = ".instance."
)

func Generate(serviceName string, instanceNumber int) string {
	return fmt.Sprintf("%s%s%d", serviceName, separator, instanceNumber)
}

func GenerateForModel(service *model.Service) string {
	return Generate(service.Sname, service.Instance)
}

func ExtractServiceName(taskId string) string {
	index := strings.LastIndex(taskId, separator)
	if index <= 0 {
		return ""
	}

	return taskId[0:index]
}

func ExtractInstanceNumber(taskId string) int {
	index := strings.LastIndex(taskId, separator)
	if index < 0 {
		return 0
	}

	instanceNumber, err := strconv.Atoi(taskId[index+len(separator):])
	if err != nil {
		return 0
	}

	return instanceNumber
}
