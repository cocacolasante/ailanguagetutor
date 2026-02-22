package handlers

import "net/http"

type Language struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	NativeName string `json:"native_name"`
	Flag       string `json:"flag"`
}

type Topic struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

var Languages = []Language{
	{Code: "it", Name: "Italian", NativeName: "Italiano", Flag: "🇮🇹"},
	{Code: "es", Name: "Spanish", NativeName: "Español", Flag: "🇪🇸"},
	{Code: "pt", Name: "Portuguese", NativeName: "Português", Flag: "🇧🇷"},
}

var Topics = []Topic{
	// Everyday Life
	{ID: "general", Name: "General Conversation", Icon: "💬", Description: "Everyday small talk, greetings, and casual chat", Category: "Everyday Life"},
	{ID: "daily-recap", Name: "Daily Recap", Icon: "📅", Description: "Recap your day, share stories and experiences", Category: "Everyday Life"},
	{ID: "future-plans", Name: "Future Plans", Icon: "🗓️", Description: "Discuss upcoming events, dreams, and goals", Category: "Everyday Life"},
	{ID: "home", Name: "Home & Living", Icon: "🏠", Description: "Household topics, décor, and neighborhoods", Category: "Everyday Life"},

	// Social
	{ID: "family", Name: "Family & Relationships", Icon: "👨‍👩‍👧", Description: "Talk about family, friends, and relationships", Category: "Social"},
	{ID: "food-dining", Name: "Food & Dining", Icon: "🍽️", Description: "Restaurants, ordering food, recipes, and cuisine", Category: "Social"},
	{ID: "shopping", Name: "Shopping", Icon: "🛍️", Description: "Stores, markets, prices, and fashion", Category: "Social"},

	// Travel & Leisure
	{ID: "travel", Name: "Travel & Tourism", Icon: "✈️", Description: "Directions, hotels, airports, and sightseeing", Category: "Travel & Leisure"},
	{ID: "sports", Name: "Sports & Fitness", Icon: "⚽", Description: "Sports, teams, gym routines, and exercise", Category: "Travel & Leisure"},
	{ID: "entertainment", Name: "Entertainment", Icon: "🎬", Description: "TV, movies, music, gaming, and pop culture", Category: "Travel & Leisure"},
	{ID: "culture", Name: "Culture & Arts", Icon: "🎭", Description: "Art, music, literature, festivals, and traditions", Category: "Travel & Leisure"},
	{ID: "environment", Name: "Environment & Nature", Icon: "🌿", Description: "Weather, ecology, and outdoor activities", Category: "Travel & Leisure"},

	// Health & Learning
	{ID: "health", Name: "Health & Wellness", Icon: "🏥", Description: "Doctor visits, fitness, symptoms, and well-being", Category: "Health & Learning"},
	{ID: "education", Name: "Education & Learning", Icon: "📚", Description: "School, courses, studying, and academic life", Category: "Health & Learning"},

	// Professional
	{ID: "work", Name: "Work & Career", Icon: "💼", Description: "Job interviews, workplace, and career development", Category: "Professional"},
	{ID: "technology", Name: "Technology", Icon: "💻", Description: "Tech talk, software, devices, and digital life", Category: "Professional"},
	{ID: "cloud", Name: "Cloud Computing", Icon: "☁️", Description: "Cloud services, DevOps, Kubernetes, and infrastructure", Category: "Professional"},
	{ID: "marketing", Name: "Marketing & Business", Icon: "📊", Description: "Campaigns, branding, sales, and business strategy", Category: "Professional"},
	{ID: "finance", Name: "Finance & Banking", Icon: "💰", Description: "Money, investments, banking, and economics", Category: "Professional"},
	{ID: "news", Name: "News & Current Events", Icon: "📰", Description: "Discussing news, politics, and world affairs", Category: "Professional"},
}

func GetLanguages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Languages)
}

func GetTopics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Topics)
}

// ── Validation helpers used by conversation handler ───────────────────────────

func IsValidLanguage(lang string) bool {
	for _, l := range Languages {
		if l.Code == lang {
			return true
		}
	}
	return false
}

func IsValidTopic(topic string) bool {
	for _, t := range Topics {
		if t.ID == topic {
			return true
		}
	}
	return false
}

func TopicDetails(topicID string) (name, description string) {
	for _, t := range Topics {
		if t.ID == topicID {
			return t.Name, t.Description
		}
	}
	return "General Conversation", "Everyday casual conversation"
}

func LanguageName(code string) string {
	for _, l := range Languages {
		if l.Code == code {
			return l.Name
		}
	}
	return "Italian"
}
