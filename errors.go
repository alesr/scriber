package scriber

var (
	// Enum errors

	errNameRequired = NameRequiredError{"name is required"}
	errExtRequired  = ExtRequiredError{"extension is required"}
	errorOutputType = OutputTypeError{"output type is not supported"}
	errorLanguage   = LanguageError{"language is required"}
	errorData       = DataError{"data is required"}
)

type (
	NameRequiredError struct{ E }
	ExtRequiredError  struct{ E }
	OutputTypeError   struct{ E }
	LanguageError     struct{ E }
	DataError         struct{ E }
)

// E is an error type that implements the error interface.
type E string

func (e E) Error() string { return string(e) }
