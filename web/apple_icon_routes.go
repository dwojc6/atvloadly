package web

import (
	"net/http"

	"github.com/bitxeno/atvloadly/internal/service"
	"github.com/gofiber/fiber/v2"
)

func registerAppleIconRoutes(api fiber.Router) {
	api.Get("/icons/apple/search", func(c *fiber.Ctx) error {
		results, err := service.SearchAppleIcons(c.UserContext(), c.Query("term"), c.Query("country"), c.Query("platform", "tvos") == "tvos")
		if err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		return c.Status(http.StatusOK).JSON(apiSuccess(results))
	})
	api.Post("/icons/apple/artwork", func(c *fiber.Ctx) error {
		var req struct {
			URL string `json:"url"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusOK).JSON(apiError("invalid artwork request"))
		}
		data, err := service.DownloadAppleIcon(c.UserContext(), req.URL)
		if err != nil {
			return c.Status(http.StatusOK).JSON(apiError(err.Error()))
		}
		return c.Status(http.StatusOK).JSON(apiSuccess(data))
	})
}
