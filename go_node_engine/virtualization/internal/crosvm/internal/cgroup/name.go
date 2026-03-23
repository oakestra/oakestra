package cgroup

import (
	"crypto/sha256"
	"encoding/base32"
	"regexp"
	"strings"
)

const machineNameMaxLen = 64
const machineNamePrefix = "oakestra."

var encoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)
var invalidMachineNameChars = regexp.MustCompile("[^a-zA-Z0-9.]")

// ConvertTaskIdToMachineName converts the given taskId of an instance to a name that can be used to create a systemd machine.
// The following requirements are fulfilled by the resulting machine name:
//   - The maximum length for machine names is 64 characters.
//   - Machine names can only contain ASCII characters and mostly only alphanumeric ones.
//     To make sure this requirement is fulfilled, only alphanumeric ASCII and "." are taken over from the taskId into the result.
//   - For more consistency in unit names, all uppercase characters in the taskId are converted to lowercase
//     when taken over into the result.
//   - To differentiate oakestra systemd machines, the result will have a "oakestra." prefix.
//   - To ensure generated machine names are still unique after all the steps taken above,
//     part of the base-32 encoded hash of the original task id is appended to the result as ".${hash}".
func ConvertTaskIdToMachineName(taskId string) string {
	hashBytes := sha256.Sum256([]byte(taskId))
	hashString := encoding.EncodeToString(hashBytes[:])
	// At this point hashString is 52 characters long which is too long to be useful, so we truncate it.
	// This should still make collisions very unlikely.
	hashString = hashString[:20]

	machineNameSuffix := "." + hashString

	safeTaskId := strings.ToLower(invalidMachineNameChars.ReplaceAllString(taskId, ""))
	safeTaskId = safeTaskId[:min(machineNameMaxLen-len(machineNameSuffix)-len(machineNamePrefix), len(safeTaskId))]

	return machineNamePrefix + safeTaskId + machineNameSuffix
}
