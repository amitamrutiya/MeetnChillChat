package handlers

import "github.com/gofiber/fiber/v2"

// render welcome page
func Welcome(c *fiber.Ctx) error {
	return c.Render("welcome", nil, "layouts/main")
}
