package main

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

const serviceNamespace = "http://foo.com/password"
const serviceURL = "http://localhost:9110/service"



// --- WSDL Definition ---

type SOAPEnvelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Body    SOAPBody `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
}

type SOAPBody struct {
	XMLName xml.Name    `xml:",omitempty"`
	Content interface{} `xml:",any"`
    Fault   *SOAPFault  `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
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

type FaultDetailWrapper struct {
	UserNotFound *UserNotFoundFaultDetail `xml:"http://foo.com/password UserNotFoundFaultDetail,omitempty"`
	UserExists   *UserExistsFaultDetail   `xml:"http://foo.com/password UserExistsFaultDetail,omitempty"` // Opraven tag
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



// --- Application logic ---

func passwordReset(username string, password string) {

	content := PasswordResetRequest{Username: username, Password: password}
	envelope := SOAPEnvelope{
		Body: SOAPBody{Content: content},
	}
	action := "http://foo.com/password/PasswordReset"
	sendRequest(envelope, action, username, password)

}

func userAdd(username string, password string) {

	content := UserAddRequest{Username: username, Password: password}
	envelope := SOAPEnvelope{
		Body: SOAPBody{Content: content},
	}
	action := "http://foo.com/password/UserAdd"
	sendRequest(envelope, action, username, password)

}



func sendRequest(requestEnvelope SOAPEnvelope, requestAction string, username string, password string) {

	xmlRequest, err := xml.MarshalIndent(requestEnvelope, "", "  ")
	if err != nil {
		log.Printf("Error marshalling request: %v", err)
		return
	}

	log.Printf("Sending SOAP Request (%s):\n%s\n", requestAction, string(xmlRequest))

	req, err := http.NewRequest(http.MethodPost, serviceURL, bytes.NewBuffer(xmlRequest))
	if err != nil {
		log.Printf("Error creating HTTP request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", requestAction)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending HTTP request: %v", err)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return
	}
	log.Printf("Received SOAP Response (Status: %d):\n%s\n", resp.StatusCode, string(bodyBytes))

	var responseEnvelope SOAPEnvelope
	responseEnvelope.Body.Content = new(interface{})                                                   // Stále problematické pro úspěch
	responseEnvelope.Body.Fault = &SOAPFault{Detail: &SOAPFaultDetail{Content: &FaultDetailWrapper{}}} // Wrapper pro Fault

	err = xml.Unmarshal(bodyBytes, &responseEnvelope)
	if err != nil {
		log.Printf("Error unmarshalling response: %v\nRaw response: %s", err, string(bodyBytes))
		return
	}

	if responseEnvelope.Body.Fault != nil && responseEnvelope.Body.Fault.FaultString != "" {
		
        faultDetailWrapper, ok := responseEnvelope.Body.Fault.Detail.Content.(*FaultDetailWrapper)
		if !ok {
			log.Printf("SOAP Fault: Code=%s, String=%s, Detail: (Failed to assert FaultDetailWrapper type: %T)",
				responseEnvelope.Body.Fault.FaultCode, responseEnvelope.Body.Fault.FaultString, responseEnvelope.Body.Fault.Detail.Content)
			return
		}

		errMsg := fmt.Sprintf("SOAP Fault: Code=%s, String=%s", responseEnvelope.Body.Fault.FaultCode, responseEnvelope.Body.Fault.FaultString)

		switch {

            case faultDetailWrapper.UserNotFound != nil:
                errMsg += fmt.Sprintf(", Detail: %s",
                    faultDetailWrapper.UserNotFound.Message)
            case faultDetailWrapper.UserExists != nil:
                errMsg += fmt.Sprintf(", Detail: %s", faultDetailWrapper.UserExists.Message)
            default:
                errMsg += ", Detail: (Unknown or missing specific detail type in wrapper)"
		
        }

		log.Printf(errMsg)

		return

	}

	if responseEnvelope.Body.Content != nil {
		log.Printf("Success Response Received (Type: %T, Value: %+v). Action: %s",
			responseEnvelope.Body.Content, responseEnvelope.Body.Content, requestAction)
	} else {
		log.Println("Received response with no fault and no content?")
	}

}

func main() {

	scanner := bufio.NewScanner(os.Stdin)

	for {

		fmt.Printf("> ")

		scanned := scanner.Scan()
		if !scanned {
			if err := scanner.Err(); err != nil {
				log.Printf("Scanner error: %v", err)
			}
			break
		}

		line := scanner.Text()
		line = strings.TrimSpace(line)

		if line == ":q" {
			break
		}

		if line == "" {
			continue
		}

		args := strings.Fields(line)
		command := args[0]
		args = args[1:]

		switch command {

		case "user":
			if len(args) < 2 {
				fmt.Println("Usage: user <username> <password>")
				continue
			}
			userAdd(args[0], args[1])

		case "pass": // Změna hesla
			if len(args) < 2 {
				fmt.Println("Usage: pass <username> <new_password>")
				continue
			}
			passwordReset(args[0], args[1])

		default:
			fmt.Println("Unknown command! Use 'user <name> <pass>' or 'pass <name> <new_pass>' or ':q' to exit.")

		}

	}

	log.Println("Client exiting.")

}

