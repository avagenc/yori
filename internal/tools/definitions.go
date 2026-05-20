// Package tools provides GenAI function declarations and HTTP-based
// tool execution for the Gmail Agent.
package tools

import (
	"google.golang.org/genai"
)

// Tool name constants — these MUST match the names the Gemini model will call.
const (
	ToolNameGmailRead               = "gmail-read"
	ToolNameGmailSend               = "gmail-send"
	ToolNameGmailGetLables          = "gmail-get-lables"
	ToolNameGmailApplyLable         = "gmail-apply-lable"
	ToolNameGmailCreateLable        = "gmail-create-lable"
	ToolNameGmailGetMessagesByLable = "gmail-get-messages-by-lable"
	ToolNameGetUserContact          = "get_user_contact"
	ToolNameAddUserContact          = "add_user_contact"
)

// GmailTools returns the genai.Tool containing all function declarations
// available to the Gmail Agent.
func GmailTools() []*genai.Tool {
	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				gmailReadDeclaration(),
				gmailSendDeclaration(),
				gmailGetLablesDeclaration(),
				gmailApplyLableDeclaration(),
				gmailCreateLableDeclaration(),
				gmailGetMessagesByLableDeclaration(),
				getUserContactDeclaration(),
				addUserContactDeclaration(),
			},
		},
	}
}

// gmailReadDeclaration defines the gmail-read tool schema.
// Retrieves emails from the user's Gmail inbox using various filters.
func gmailReadDeclaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name: ToolNameGmailRead,
		Description: `Retrieve emails from the user's Gmail inbox using various filters such as sender, query, labels, and date range. 
Use this when the user asks to read, search, or view emails. 
Return the email summary or required information from the result.`,
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"query": {
					Type:        genai.TypeString,
					Description: "Free text search keyword applied to email fields (like Gmail search). Use empty string if not needed.",
				},
				"from": {
					Type:        genai.TypeString,
					Description: "Filter by sender email address. Use empty string if not needed.",
				},
				"label": {
					Type:        genai.TypeString,
					Description: "Gmail label name or ID to filter by. Use empty string if not needed.",
				},
				"after": {
					Type:        genai.TypeString,
					Description: "Show emails AFTER this date. Format: YYYY/MM/DD. Use empty string if not needed.",
				},
				"before": {
					Type:        genai.TypeString,
					Description: "Show emails BEFORE this date. Format: YYYY/MM/DD. Use empty string if not needed.",
				},
				"max_results": {
					Type:        genai.TypeString,
					Description: "Maximum number of emails to return as a string integer. Default is '10'.",
				},
			},
			Required: []string{"query", "from", "label", "after", "before", "max_results"},
		},
	}
}

// gmailSendDeclaration defines the gmail-send tool schema.
// Sends an email on behalf of the user via Gmail.
func gmailSendDeclaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        ToolNameGmailSend,
		Description: "Send an email on behalf of the user via Gmail. All three fields (receiver, title, body) are required. Do NOT call this unless all fields are present.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"receiver": {
					Type:        genai.TypeString,
					Description: "The recipient's email address (REQUIRED).",
				},
				"title": {
					Type:        genai.TypeString,
					Description: "The email subject line (REQUIRED).",
				},
				"body": {
					Type:        genai.TypeString,
					Description: "The email body text in plain text (REQUIRED).",
				},
			},
			Required: []string{"receiver", "title", "body"},
		},
	}
}

// gmailGetLablesDeclaration defines the gmail-get-lables tool schema.
// Fetches all of user's Gmail labels.
func gmailGetLablesDeclaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        ToolNameGmailGetLables,
		Description: "Fetch all of the user's Gmail labels. Returns label IDs and names. No additional parameters required beyond authentication.",
		Parameters: &genai.Schema{
			Type:       genai.TypeObject,
			Properties: map[string]*genai.Schema{},
		},
	}
}

// gmailApplyLableDeclaration defines the gmail-apply-lable tool schema.
// Applies a label to a specific message.
func gmailApplyLableDeclaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        ToolNameGmailApplyLable,
		Description: "Apply a label to a specific Gmail message. Requires the message_id (obtained from gmail-read) and label_id (obtained from gmail-get-lables).",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"message_id": {
					Type:        genai.TypeString,
					Description: "The unique message ID from gmail-read results.",
				},
				"label_id": {
					Type:        genai.TypeString,
					Description: "The label ID from gmail-get-lables results (e.g., 'Label_1234').",
				},
			},
			Required: []string{"message_id", "label_id"},
		},
	}
}

// gmailCreateLableDeclaration defines the gmail-create-lable tool schema.
// Creates a new Gmail label.
func gmailCreateLableDeclaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        ToolNameGmailCreateLable,
		Description: "Create a new Gmail label in the user's account with a custom name.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"name": {
					Type:        genai.TypeString,
					Description: "The name for the new label (REQUIRED).",
				},
				"label_list_visibility": {
					Type:        genai.TypeString,
					Description: "Visibility in label list: 'labelShow' or 'labelHide'. Use empty string for default.",
				},
				"message_list_visibility": {
					Type:        genai.TypeString,
					Description: "Visibility of labeled messages: 'show' or 'hide'. Use empty string for default.",
				},
				"text_color": {
					Type:        genai.TypeString,
					Description: "HEX color code for label text (e.g., '#ffffff'). Use empty string for default.",
				},
				"background_color": {
					Type:        genai.TypeString,
					Description: "HEX color code for label background (e.g., '#0a539b'). Use empty string for default.",
				},
			},
			Required: []string{"name"},
		},
	}
}

// gmailGetMessagesByLableDeclaration defines the gmail-get-messages-by-lable tool schema.
// Gets messages by a specific label ID.
func gmailGetMessagesByLableDeclaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        ToolNameGmailGetMessagesByLable,
		Description: "Fetch Gmail messages filtered by a specific label. Requires label_id obtained from gmail-get-lables.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"label_id": {
					Type:        genai.TypeString,
					Description: "The label ID to search for messages (e.g., 'Label_1234').",
				},
				"max_results": {
					Type:        genai.TypeString,
					Description: "Maximum number of messages to return as a string integer. Default is '10'.",
				},
			},
			Required: []string{"label_id"},
		},
	}
}

// getUserContactDeclaration defines the get_user_contact tool schema.
func getUserContactDeclaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        ToolNameGetUserContact,
		Description: "Get the user's contact list including Gmail addresses and phone numbers. Use this when a name is provided but the Gmail address or phone number is not provided and needs to be looked up.",
		Parameters: &genai.Schema{
			Type:       genai.TypeObject,
			Properties: map[string]*genai.Schema{},
		},
	}
}

// addUserContactDeclaration defines the add_user_contact tool schema.
func addUserContactDeclaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        ToolNameAddUserContact,
		Description: "Add a new user contact with Gmail address and/or phone number. Use this when the command tells you to add a contact or when a Gmail/phone is provided but the contact is not yet in the list. Name is required, and at least one of gmail or number must be provided.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"name": {
					Type:        genai.TypeString,
					Description: "The name of the contact (REQUIRED).",
				},
				"gmail": {
					Type:        genai.TypeString,
					Description: "The Gmail address of the contact. Use empty string if not provided.",
				},
				"number": {
					Type:        genai.TypeString,
					Description: "The phone number of the contact. Use empty string if not provided.",
				},
			},
			Required: []string{"name"},
		},
	}
}
