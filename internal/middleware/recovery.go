// Package middleware provides HTTP middleware components
package middleware

import (
	"fmt"
	"net/http"
	"runtime"
)

// Recovery recovers from panics and returns a 500 error
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				stack := string(buf[:n])

				// TODO: Use structured logger
				fmt.Printf("PANIC: %v\n%s\n", err, stack)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
