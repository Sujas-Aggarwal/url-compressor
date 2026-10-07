package helper

import (
	"fmt"
	"reflect"
)

func PrintHelpMessage() {
	fmt.Println("<usage> urlshortener encode $URL")
	fmt.Println("<usage> urlshortener decode $HASH")
}

func CustomStructPrinter(obj interface{}) {
	val := reflect.ValueOf(obj)
	typ := reflect.TypeOf(obj)

	// If a pointer is passed, get the underlying element
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
		typ = typ.Elem()
	}

	// Ensure we are dealing with a struct
	if val.Kind() != reflect.Struct {
		fmt.Println("Provided value is not a struct")
		return
	}

	fmt.Printf("--- Struct: %s ---\n", typ.Name())
	for i := 0; i < val.NumField(); i++ {
		fieldName := typ.Field(i).Name
		fieldValue := val.Field(i)

		// Custom formatting logic per field
		fmt.Printf(" => %-10s : %v\n", fieldName, fieldValue)
	}
	fmt.Println("-----------------------")
}
