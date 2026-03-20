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

type Personality struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
}

var Personalities = []Personality{
	{ID: "professor", Name: "Professor", Icon: "🎓", Description: "Formal, structured, precise. Excellent grammar explanations."},
	{ID: "friendly-partner", Name: "Friendly Partner", Icon: "😊", Description: "Casual, warm, encouraging. Relaxed conversation practice."},
	{ID: "bartender", Name: "Bartender", Icon: "🍺", Description: "Laid-back, witty, authentic. Everyday slang and expressions."},
	{ID: "business-executive", Name: "Business Executive", Icon: "💼", Description: "Professional, direct, formal. Business vocabulary focus."},
	{ID: "travel-guide", Name: "Travel Guide", Icon: "🗺️", Description: "Enthusiastic, cultural, storytelling. Local phrases and culture."},
}

var Languages = []Language{
	{Code: "it", Name: "Italian", NativeName: "Italiano", Flag: "🇮🇹"},
	{Code: "es", Name: "Spanish", NativeName: "Español", Flag: "🇪🇸"},
	{Code: "pt", Name: "Portuguese", NativeName: "Português", Flag: "🇧🇷"},
	{Code: "en", Name: "English", NativeName: "English", Flag: "🇺🇸"},
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

	// Role-Play Scenarios
	{ID: "role-restaurant", Name: "Restaurant Ordering", Icon: "🍽️", Description: "Order food, ask about the menu, and pay the bill", Category: "Role-Play Scenarios"},
	{ID: "role-job-interview", Name: "Job Interview", Icon: "👔", Description: "Practice professional interviews and workplace language", Category: "Role-Play Scenarios"},
	{ID: "role-airport", Name: "Airport & Travel", Icon: "✈️", Description: "Check in, security, boarding, and asking for help", Category: "Role-Play Scenarios"},
	{ID: "role-doctor", Name: "Doctor Visit", Icon: "🏥", Description: "Describe symptoms, understand medical advice", Category: "Role-Play Scenarios"},
	{ID: "role-business", Name: "Business Meeting", Icon: "💼", Description: "Negotiate, present ideas, and follow business etiquette", Category: "Role-Play Scenarios"},
	{ID: "role-apartment", Name: "Renting an Apartment", Icon: "🏠", Description: "View apartments, negotiate rent, sign agreements", Category: "Role-Play Scenarios"},
	{ID: "role-directions", Name: "Asking Directions", Icon: "🗺️", Description: "Navigate a city, understand landmarks and transit", Category: "Role-Play Scenarios"},

	// Immersion Mode
	{ID: "immersion-daily", Name: "Daily Life", Icon: "🏡", Description: "Navigate everyday situations — shopping, transport, home, errands", Category: "Immersion Mode"},
	{ID: "immersion-social", Name: "Social Scene", Icon: "🥂", Description: "A party, dinner with friends, casual social encounters with native speakers", Category: "Immersion Mode"},
	{ID: "immersion-work", Name: "Workplace", Icon: "💼", Description: "Meetings, colleagues, and office conversations entirely in the target language", Category: "Immersion Mode"},
	{ID: "immersion-city", Name: "City Exploration", Icon: "🏙️", Description: "Ask for directions, explore neighborhoods, navigate public transport", Category: "Immersion Mode"},
	{ID: "immersion-media", Name: "Film & Music", Icon: "🎬", Description: "Discuss movies, TV shows, songs, and pop culture as a native would", Category: "Immersion Mode"},
	{ID: "immersion-debate", Name: "Opinion & Debate", Icon: "🗣️", Description: "Share views, argue positions, and engage in real native-level discourse", Category: "Immersion Mode"},

	// Cultural Language Learning
	{ID: "cultural-context", Name: "Cultural Context Lessons", Icon: "🏛️", Description: "Learn social norms, etiquette, and unwritten rules through guided cultural discussion", Category: "Cultural Language Learning"},
	{ID: "cultural-stories", Name: "Story-Based Learning", Icon: "📖", Description: "Immerse yourself in short authentic stories set in real cultural contexts", Category: "Cultural Language Learning"},
	{ID: "cultural-idioms", Name: "Idioms & Expressions", Icon: "💬", Description: "Master common idioms, proverbs, and sayings with their cultural origins", Category: "Cultural Language Learning"},
	{ID: "cultural-food", Name: "Food & Cuisine Culture", Icon: "🍜", Description: "Explore food traditions, dining customs, and culinary vocabulary", Category: "Cultural Language Learning"},
	{ID: "cultural-history", Name: "History & Traditions", Icon: "🎭", Description: "Discover festivals, historical context, and regional cultural traditions", Category: "Cultural Language Learning"},

	// Grammar & Skills
	{ID: "grammar-vocabulary", Name: "Vocabulary Builder", Icon: "📚", Description: "Learn new words through guided exercises, phonetic breakdowns, and translation quizzes", Category: "Grammar & Skills"},
	{ID: "grammar-sentences", Name: "Sentence Construction", Icon: "✏️", Description: "Build grammatically correct sentences through word-order and fill-in-the-blank exercises", Category: "Grammar & Skills"},
	{ID: "grammar-pronunciation", Name: "Pronunciation Practice", Icon: "🗣️", Description: "Perfect your pronunciation with phonetic breakdowns, stress guides, and sound drills", Category: "Grammar & Skills"},
	{ID: "grammar-listening", Name: "Listening Comprehension", Icon: "👂", Description: "Improve listening skills through short passages and comprehension questions", Category: "Grammar & Skills"},
	{ID: "grammar-writing", Name: "Writing Coach", Icon: "📝", Description: "Submit writing for detailed grammar corrections, style upgrades, and encouragement", Category: "Grammar & Skills"},

	// AI Travel Mode
	{ID: "travel-rome", Name: "Rome, Italy", Icon: "🇮🇹", Description: "Explore Rome: food, art, navigation, and local culture", Category: "AI Travel Mode"},
	{ID: "travel-barcelona", Name: "Barcelona, Spain", Icon: "🇪🇸", Description: "Navigate Barcelona's tapas bars, beaches, and architecture", Category: "AI Travel Mode"},
	{ID: "travel-paris", Name: "Paris, France", Icon: "🇫🇷", Description: "Paris café culture, museums, and everyday Parisian life", Category: "AI Travel Mode"},
	{ID: "travel-tokyo", Name: "Tokyo, Japan", Icon: "🇯🇵", Description: "Tokyo's subway, restaurants, and cultural etiquette", Category: "AI Travel Mode"},
	{ID: "travel-lisbon", Name: "Lisbon, Portugal", Icon: "🇵🇹", Description: "Lisbon's neighborhoods, trams, and traditional cuisine", Category: "AI Travel Mode"},
}

func GetLanguages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Languages)
}

func GetTopics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Topics)
}

func GetPersonalities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Personalities)
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

func IsValidPersonality(personality string) bool {
	if personality == "" {
		return true
	}
	for _, p := range Personalities {
		if p.ID == personality {
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
