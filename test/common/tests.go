package common

var (
	ALL_TESTS = []TestCase{}

	AFTER_INSTALL_TESTS = []TestCase{
		PodDisruptionBudgetTest,
	}
)
