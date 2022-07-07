package virtualization

import (
	"gotest.tools/assert"
	"testing"
)

func TestSnameExtractionFromTaskid(t *testing.T) {
	sname := "test.test.nginx.test"
	taskid := genTaskID(sname, 23)
	extracted := extractSnameFromTaskID(taskid)
	assert.Equal(t, extracted, sname)
}

func TestSnameExtractionFromTaskidMultipleInstance(t *testing.T) {
	sname := "test.test.instance.test"
	taskid := genTaskID(sname, 23)
	extracted := extractSnameFromTaskID(taskid)
	assert.Equal(t, extracted, sname)
}

func TestInstanceExtractionFromTaskid(t *testing.T) {
	sname := "test.test.nginx.test"
	iid := 23
	taskid := genTaskID(sname, iid)
	extracted := extractInstanceNumberFromTaskID(taskid)
	assert.Equal(t, extracted, iid)
}

func TestInstanceExtractionFromTaskidMultipleInstance(t *testing.T) {
	sname := "test.test.instance.test"
	iid := 23
	taskid := genTaskID(sname, iid)
	extracted := extractInstanceNumberFromTaskID(taskid)
	assert.Equal(t, extracted, iid)
}
