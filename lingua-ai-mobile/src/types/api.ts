export interface User {
  id: string;
  email: string;
  username: string;
  is_admin: boolean;
  approved: boolean;
  email_verified: boolean;
  subscription_status: 'trialing' | 'active' | 'past_due' | 'cancelled' | 'free' | 'suspended' | 'beta_trial' | '';
  trial_ends_at: string | null;
  pref_language: string;
  pref_level: number;
  pref_personality: string;
}

export interface ConversationRecord {
  id: string;
  user_id: string;
  session_id: string;
  language: string;
  topic: string;
  topic_name: string;
  level: number;
  personality: string;
  message_count: number;
  duration_secs: number;
  fp_earned: number;
  summary: string;
  topics_discussed: string[];
  vocabulary_learned: string[];
  grammar_corrections: string[];
  suggested_next_lessons: string[];
  misspellings: string[];
  created_at: string;
  ended_at: string;
}

export interface UserStats {
  streak: number;
  last_activity_date: string;
  total_fp: number;
  language_fp: Record<string, number>;
  language_level: Record<string, number>;
  achievements: string[];
  conversation_count: number;
  recent_conversations: ConversationRecord[];
}

export interface SessionStart {
  session_id: string;
  language: string;
  topic: string;
  topic_name: string;
  level: number;
  personality: string;
}

export interface EndConversationResponse {
  record_id: string;
  fp_earned: number;
  new_streak: number;
  new_achievements: string[];
  total_fp: number;
  summary: string;
  topics_discussed: string[];
  vocabulary_learned: string[];
  grammar_corrections: string[];
  suggested_next_lessons: string[];
  language: string;
  topic: string;
  topic_name: string;
  level: number;
  personality: string;
  message_count: number;
  duration_secs: number;
}

export interface Language {
  code: string;
  name: string;
  native_name: string;
  flag: string;
}

export interface Topic {
  id: string;
  name: string;
  icon: string;
  description: string;
  category: string;
}

export interface Personality {
  id: string;
  name: string;
  icon: string;
  description: string;
}

export interface BillingStatus {
  subscription_status: string;
  trial_ends_at: string | null;
  has_full_access: boolean;
  has_conversation_access: boolean;
}

export interface LeaderboardEntry {
  rank: number;
  username: string;
  total_fp: number;
  streak: number;
}

export interface Badge {
  id: string;
  name: string;
  icon: string;
  desc: string;
}
