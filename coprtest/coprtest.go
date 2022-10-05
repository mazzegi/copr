package coprtest

const (
	TestActionCrash  = "crash"
	TestActionStress = "stress"
	TestActionProbe  = "probe"
)

type TestCommand struct {
	Action string
}
