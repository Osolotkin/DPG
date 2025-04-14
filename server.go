package main

import (
	"encoding/xml"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"
)

const (
	ERR_USER_NOT_FOUND = iota
	ERR_USER_EXISTS
)

const servicePort = "9110"



// --- WSDL Definition ---

type SOAPEnvelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Body    SOAPBody `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
}

type SOAPBody struct {
	XMLName            xml.Name     `xml:",omitempty"`
	RequestBodyWrapper `xml:",any"`
	Fault              *SOAPFault   `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
}

type SOAPFault struct {
	XMLName     xml.Name         `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault"`
	FaultCode   string           `xml:"faultcode,omitempty"`
	FaultString string           `xml:"faultstring,omitempty"`
	Detail      *SOAPFaultDetail `xml:"detail,omitempty"`
}

type SOAPFaultDetail struct {
	XMLName xml.Name    `xml:"detail"`
	Content interface{} `xml:",any"`
}



// --- Local Definitions ---

type RequestBodyWrapper struct {
	PasswordReset *PasswordResetRequest `xml:"http://foo.com/password PasswordResetRequest,omitempty"`
	UserAdd       *UserAddRequest       `xml:"http://foo.com/password UserAddRequest,omitempty"`
}

type PasswordResetRequest struct {
	XMLName  xml.Name `xml:"http://foo.com/password PasswordResetRequest"`
	Username string   `xml:"username"`
	Password string   `xml:"password"`
}

type PasswordResetResponse struct {
	XMLName   xml.Name `xml:"http://foo.com/password PasswordResetResponse"`
	SessionId string   `xml:"sessionId"`
}

type UserAddRequest struct {
	XMLName  xml.Name `xml:"http://foo.com/password UserAddRequest"`
	Username string   `xml:"username"`
	Password string   `xml:"password"`
}

type UserAddResponse struct {
	XMLName   xml.Name `xml:"http://foo.com/password UserAddResponse"`
	SessionId string   `xml:"sessionId"`
}

type UserNotFoundFaultDetail struct {
	XMLName xml.Name `xml:"http://foo.com/password UserNotFoundFaultDetail"`
	Message string   `xml:"message"`
}

type UserExistsFaultDetail struct {
	XMLName xml.Name `xml:"http://foo.com/password UserExistsFaultDetail"`
    Message string   `xml:"message"`
}

type UserData struct {
	Password string
}

var users = map[string]*UserData{
	"root": {Password: "root"},
}



// --- Application logic ---

func passwordReset(w http.ResponseWriter, username string, password string) {

	data, exists := users[username]

	if !exists {
		sendSOAPFault(w, "Client", "User '"+username+"' not found!",
			&UserNotFoundFaultDetail{Message: "User '" + username + "' not found."})
		return
	}

	log.Printf("Password change for user %s: %s -> %s", username, data.Password, password)
	data.Password = password

	sessionID := uuid.New().String()
	responseContent := PasswordResetResponse{
		SessionId: sessionID}
	sendSOAPResponse(w, responseContent)

}

func userAdd(w http.ResponseWriter, username string, password string) {

	_, exists := users[username]

	if exists {
		sendSOAPFault(w, "Client", "User '"+username+"' already exists!",
			&UserExistsFaultDetail{Message: "User '" + username + "' already exists."})
		return
	}

	log.Printf("Adding new user: %s", username)
	users[username] = &UserData{ Password: password }

	sessionID := uuid.New().String()
	responseContent := UserAddResponse{ SessionId: sessionID }
	sendSOAPResponse(w, responseContent)

}



// --- SOAP functions ---

func handleSOAPRequest(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		sendSOAPFault(w, "Server", "Error reading request", nil)
		return
	}
	defer r.Body.Close()

	var requestEnvelope SOAPEnvelope

	err = xml.Unmarshal(bodyBytes, &requestEnvelope)
	if err != nil {
		log.Printf("Error unmarshalling SOAP envelope: %v", err)
		log.Printf("Received body: %s", string(bodyBytes))
		sendSOAPFault(w, "Client", "Invalid SOAP request format", nil)
		return
	}

	switch {

        case requestEnvelope.Body.PasswordReset != nil:
            req := requestEnvelope.Body.PasswordReset
            log.Printf("Received PasswordResetRequest for user: %s", req.Username)
            passwordReset(w, req.Username, req.Password)

        case requestEnvelope.Body.UserAdd != nil:
            req := requestEnvelope.Body.UserAdd
            log.Printf("Received UserAddRequest for username: %s", req.Username)
            userAdd(w, req.Username, req.Password)

        default:
            log.Printf("Received unknown or empty request content in SOAP body: %+v", requestEnvelope.Body.RequestBodyWrapper)
            sendSOAPFault(w, "Client", "Unsupported operation or empty request", nil)

	}

}

func sendSOAPResponse(w http.ResponseWriter, content interface{}) {

	// whatever
	type Body struct {
		XMLName xml.Name    `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
		Content interface{} `xml:",any"`
	}
	type Envelope struct {
		XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
		Body    Body
	}

	response := Envelope{
		Body: Body{
			Content: content,
		},
	}

	writeXMLResponse(w, http.StatusOK, response)

}

func sendSOAPFault(w http.ResponseWriter, faultCode, faultString string, detailContent interface{}) {

	fault := SOAPFault{
		FaultCode:   faultCode,
		FaultString: faultString,
	}

	if detailContent != nil {
		fault.Detail = &SOAPFaultDetail{Content: detailContent}
	}

	response := SOAPEnvelope{
		Body: SOAPBody{
			Fault: &fault,
		},
	}

	log.Printf("Sending Fault: Code=%s, String=%s, Detail=%+v", faultCode, faultString, detailContent)
	writeXMLResponse(w, http.StatusInternalServerError, response)

}

func writeXMLResponse(w http.ResponseWriter, statusCode int, envelope interface{}) {

	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(statusCode)

	encoder := xml.NewEncoder(w)

	if err := encoder.Encode(envelope); err != nil {
		log.Printf("Error encoding SOAP response: %v", err)
	}

}

func main() {

	http.HandleFunc("/service", handleSOAPRequest)

	log.Println("Starting SOAP server on http://localhost:" + servicePort + "/service")
	err := http.ListenAndServe(":"+servicePort, nil)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

}
