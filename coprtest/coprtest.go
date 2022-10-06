package coprtest

const (
	TestActionCrash  = "crash"
	TestActionStress = "stress"
	TestActionProbe  = "probe"
	TestActionGetEnv = "getenv"
)

type TestCommand struct {
	Action string
	Param  string
}
