//go:build !darwin

package beep

func Beep() {
	print("\a")
}
