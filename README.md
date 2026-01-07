# `errors`

The `errors` module augments Go's standard `errors` package to capture stack traces and associate HTTP status codes and arbitrary properties with errors. For this purpose, it overrides and extends the standard `errors.New` constructor.

```go
import "github.com/microbus-io/errors" // instead of "errors"

err := errors.New("my error") // err is augmented with the stack trace of this line

err = errors.New("error in process '%s'", processName) // err is augmented with the stack trace of this line
```

The `errors` module redefines all the methods in Go's standard `errors` package which allow seamlessly replacing `import "errors"` with `import "github.com/microbus-io/errors"`.

`errors.Trace` augments standard Go `error`s with the stack trace.

```go
import "github.com/microbus-io/errors"

fileName := "non/existent.file"
body, err := io.ReadAll(fileName) // err is a standard Go error
err = errors.Trace(err) // err is now augmented with the stack trace of this line
```

Alternatively, `errors.New` can be used to wrap the error as if with `fmt.Errof("message: %w", err)` before augmenting it with the stack trace.

```go
import "github.com/microbus-io/errors"

fileName := "non/existent.file"
body, err := io.ReadAll(fileName) // err is a standard Go error
err = errors.New("failed to read %s", fileName, err) // err now wraps the original err and captures the stack trace
```

An HTTP status code can be attached to an error with either `errors.New` or `errors.Trace`. It can then be retrieved up the stack to respond to HTTP requests.

```go
notFound := errors.New("nothing to see here", http.StatusNotFound)

body, err := io.ReadAll("non/existent.file") // err is a standard Go error
err = errors.Trace(err, http.StatusNotFound) // err is now augmented with the stack trace of this line and the status code

statusCode := errors.StatusCode(err) // Retrieve the status code
```

Both `errors.New` and `errors.Trace` allow associating arbitrary properties with errors. Properties are not part of the error's `Error()` message. Rather, they can be retrieved up the call stack in a structured way. A common use case is associating an error code or a human-friendly message, and using a middleware to render custom error responses.

```go
fileName := "non/existent.file"
body, err := io.ReadAll(fileName) // err is a standard Go error
err = errors.Trace(err
	"name", fileName, // err is now augmented with a property
)

fileName := errors.Convert(err).Properties["name"] // Retrieve the property
```

The `fmt` verb `%v` is equivalent to the `err.Error()` message and prints the error message.
The extended verb `%+v` is equivalent to `errors.Convert(err).String()` and print the stack trace, status code and associated properties.

```
strconv.ParseInt: parsing "nan": invalid syntax
statusCode=400

- calculator.(*Service).Square
  /src/github.com/microbus-io/fabric/examples/calculator/service.go:75
- connector.(*Connector).onRequest
  /src/github.com/microbus-io/fabric/connector/messaging.go:225
- connector.(*Connector).Publish
  /src/github.com/microbus-io/fabric/connector/messaging.go:94
- httpingress.(*Service).ServeHTTP
  /src/github.com/microbus-io/fabric/coreservices/httpingress/service.go:124
```

`CatchPanic` is a utility function that converts panics into standard errors.

```go
err = errors.CatchPanic(func() error {
    panic("oops!")
    return nil
})
```
