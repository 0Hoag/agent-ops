package auth

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type LoginResponse struct {
	Token string `json:"token"`
}
type SessionResponse struct {
	AccountID string `json:"account_id"`
	Email     string `json:"email"`
}
