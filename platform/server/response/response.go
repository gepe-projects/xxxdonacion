package response

import "github.com/labstack/echo/v5"

// Body adalah envelope standar response sukses.
type Body struct {
	Data any `json:"data"`
}

func JSON(c *echo.Context, status int, data any) error {
	return c.JSON(status, Body{Data: data})
}
