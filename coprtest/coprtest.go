package coprtest

const (
	TestActionCrash  = "crash"
	TestActionStress = "stress"
)

type TestCommand struct {
	Action string
}
