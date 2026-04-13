package site

import (
	"strings"
	"time"
)

func (s *Site) nutritionMealCatalog() []nutritionMealCard {
	base := append([]nutritionMealCard(nil), nutritionMealLibrary()...)
	seen := map[string]bool{}
	for _, meal := range base {
		seen[strings.TrimSpace(meal.ID)] = true
	}

	rows, err := s.DB.Query(
		`select meal_id,
		        name,
		        coalesce(description, ''),
		        coalesce(category, ''),
		        coalesce(calories, 0),
		        coalesce(protein, 0),
		        coalesce(carbs, 0),
		        coalesce(fats, 0)
		 from nutrition_custom_meals
		 where active = true
		 order by created_at desc`,
	)
	if err != nil {
		return base
	}
	defer rows.Close()

	for rows.Next() {
		var meal nutritionMealCard
		if scanErr := rows.Scan(
			&meal.ID,
			&meal.Name,
			&meal.Description,
			&meal.Category,
			&meal.Calories,
			&meal.Protein,
			&meal.Carbs,
			&meal.Fats,
		); scanErr != nil {
			continue
		}
		meal.ID = strings.TrimSpace(meal.ID)
		meal.Name = strings.TrimSpace(meal.Name)
		slot := normalizeNutritionSlotKey(meal.Category)
		if meal.ID == "" || meal.Name == "" || slot == "" {
			continue
		}
		if seen[meal.ID] {
			continue
		}
		meal.Category = nutritionSlotLabel(slot)
		base = append(base, meal)
		seen[meal.ID] = true
	}
	return base
}

func (s *Site) nutritionMealByID(id string) (nutritionMealCard, bool) {
	needle := strings.TrimSpace(id)
	if needle == "" {
		return nutritionMealCard{}, false
	}
	for _, meal := range s.nutritionMealCatalog() {
		if meal.ID == needle {
			return meal, true
		}
	}
	return nutritionMealCard{}, false
}

func (s *Site) generateNutritionCustomMealID() string {
	for i := 0; i < 5; i++ {
		token, err := randomToken(9)
		if err != nil {
			continue
		}
		token = strings.ToLower(strings.TrimSpace(token))
		token = strings.ReplaceAll(token, "-", "")
		token = strings.ReplaceAll(token, "_", "")
		if token == "" {
			continue
		}
		return "meal-custom-" + token
	}
	return "meal-custom-" + strings.ReplaceAll(time.Now().Format("20060102150405.000000000"), ".", "")
}

func nutritionMealsBySlotFromCatalog(slotKey string, catalog []nutritionMealCard) []nutritionMealCard {
	category := nutritionSlotLabel(slotKey)
	if category == "" {
		return nil
	}
	meals := make([]nutritionMealCard, 0, len(catalog))
	for _, meal := range catalog {
		if strings.EqualFold(strings.TrimSpace(meal.Category), category) {
			meals = append(meals, meal)
		}
	}
	return meals
}

func nutritionFallbackMealForSlotFromCatalog(slotKey string, catalog []nutritionMealCard) nutritionMealCard {
	candidates := nutritionMealsBySlotFromCatalog(slotKey, catalog)
	if len(candidates) > 0 {
		return candidates[0]
	}
	category := nutritionSlotLabel(slotKey)
	if category == "" {
		category = "Перекус"
	}
	return nutritionMealCard{ID: "fallback", Name: "Блюдо по умолчанию", Category: category}
}
