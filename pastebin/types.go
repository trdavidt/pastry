package pastebin

const (
	apiPost     = "/api/api_post.php"
	apiRaw      = "/api/api_raw.php"
	apiLogin    = "/api/api_login.php"
	apiRawPaste = "/raw/"
)

// CreateOptions holds the optional parameters for creating a paste.
type CreateOptions struct {
	Title   string
	Format  string
	Expire  string
	Private string
	Code    string
	UserKey string
	Folder  string
}

// Paste is one entry in the list response from the pastebin API.
type Paste struct {
	Key         string `xml:"paste_key"`
	Date        string `xml:"paste_date"`
	Title       string `xml:"paste_title"`
	Size        string `xml:"paste_size"`
	ExpireDate  string `xml:"paste_expire_date"`
	Private     string `xml:"paste_private"`
	FormatLong  string `xml:"paste_format_long"`
	FormatShort string `xml:"paste_format_short"`
	URL         string `xml:"paste_url"`
	Hits        string `xml:"paste_hits"`
}

// User is the userdetails response from the pastebin API.
type User struct {
	Name        string `xml:"user_name"`
	FormatShort string `xml:"user_format_short"`
	Expiration  string `xml:"user_expiration"`
	AvatarURL   string `xml:"user_avatar_url"`
	Private     string `xml:"user_private"`
	Website     string `xml:"user_website"`
	Email       string `xml:"user_email"`
	Location    string `xml:"user_location"`
	AccountType string `xml:"user_account_type"`
}
