package theater

// Version returns the Theater build version embedded in the current binary.
func Version() string {
	return version
}

var version = "dev"
