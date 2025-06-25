package cgroup

import (
	"crypto/sha256"
	"encoding/base32"
	"regexp"
	"strings"
)

const machineUnitMaxLen = 255
const machineUnitPrefix = "machine-"
const machineUnitSuffix = ".scope"
const machineNamePrefix = "oakestra."
const machinePreservedLen = len(machineUnitPrefix) + len(machineUnitSuffix) + len(machineNamePrefix)

var encoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)
var invalidMachineNameChars = regexp.MustCompile("[^a-zA-Z0-9.]")

// ConvertTaskIdToMachineName converts the given taskId of an instance to a name that can be used to create a systemd machine.
// The following requirements are fulfilled by the resulting machine name:
//   - The name of the resulting systemd unit has the format "machine-${machine-name}.scope"
//     and the maximum length for systemd unit names is 255 characters.
//     So the result of this function is truncated to a maximum length of `255 - (len("machine-") + len(".scope"))`.
//   - Systemd unit names can only contain ASCII characters and mostly only alphanumeric ones.
//     To make sure this requirement is fulfilled, only alphanumeric ASCII and "." are
//     taken over from the taskId into the result.
//   - For more consistency in unit names, all uppercase characters in the taskId are converted to lowercase
//     when taken over into the result.
//   - To differentiate oakestra systemd machines, the result will have a "oakestra." prefix.
//   - To ensure generated machine names are still unique after all the steps taken above,
//     a base-32 encoded hash of the original task id is appended to the result as ".${hash}".
//     Truncation is never applied to the hash only everything before it.
func ConvertTaskIdToMachineName(taskId string) string {
	hashBytes := sha256.Sum256([]byte(taskId))
	hashString := encoding.EncodeToString(hashBytes[:])
	machineNameSuffix := "." + hashString

	safeTaskId := strings.ToLower(invalidMachineNameChars.ReplaceAllString(taskId, ""))
	safeTaskId = safeTaskId[:min(machineUnitMaxLen-len(machineNameSuffix)-machinePreservedLen, len(safeTaskId))]

	return machineNamePrefix + safeTaskId + machineNameSuffix
}
