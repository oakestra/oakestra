package image

type Source interface {
	Name() string
	Retrieve(id string, dstDirPath string) error
}
