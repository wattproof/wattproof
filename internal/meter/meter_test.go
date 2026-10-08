package meter

import "testing"

func TestHeadline(t *testing.T) {
	want := map[Class]bool{ClassA: true, ClassB: true, ClassC: false, ClassD: false, Class(0): false}
	for c, w := range want {
		if got := c.Headline(); got != w {
			t.Errorf("%v.Headline() = %v, want %v", c, got, w)
		}
	}
}

func TestClassString(t *testing.T) {
	for c, w := range map[Class]string{ClassA: "A", ClassD: "D", Class(9): "Class(9)"} {
		if got := c.String(); got != w {
			t.Errorf("String() = %q, want %q", got, w)
		}
	}
}
