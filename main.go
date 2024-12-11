package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Component struct {
	Name        string `json:"name"`
	Epoch       string `json:"epoch"`
	Version     string `json:"version"`
	Release     string `json:"release"`
	Arch        string `json:"arch"`
	Installtime string `json:"installtime"`
	Buildtime   string `json:"buildtime"`
	Vendor      string `json:"vendor"`
	Buildhost   string `json:"buildhost"`
	Sigpgp      string `json:"sigpgp"`
}

type SystemComponents struct {
	SystemID   string      `json:"systemid"`
	Components []Component `json:"components"`
}

type SystemComponentList struct {
	Systems []SystemComponents `json:"systems"`
}

var systemComponents []SystemComponents

func untar(reader io.Reader, filesInMemory map[string]*bytes.Buffer) error {
	// Create a new tar reader
	tarReader := tar.NewReader(reader)

	// Loop over the files in the tarball
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of tar archive
		}
		if err != nil {
			return fmt.Errorf("error reading tar file: %v", err)
		}

		// Check if the entry is a directory or a file
		switch header.Typeflag {
		case tar.TypeDir:
			// Create a directory in-memory (directories aren't actually stored in-memory, but we could track them if necessary)
			// For simplicity, we just skip directories in this case, as we don't need to store them.
			// In case we need to simulate directories, we could use empty buffers for them.
			continue
		case tar.TypeReg:
			// Create a buffer for the file content in memory
			buf := new(bytes.Buffer)

			// Copy the file content from the tar reader into the in-memory buffer
			_, err := io.Copy(buf, tarReader)
			if err != nil {
				return fmt.Errorf("error reading file content: %v", err)
			}

			// Take off the custom archive name as the root folder
			fileName := strings.SplitAfterN(header.Name, "/", 2)[1]
			// Store the file's content in the map
			filesInMemory[fileName] = buf
		}
	}

	return nil
}

func extractPackages(files map[string]*bytes.Buffer) error {
	packageListFileName := "data/insights_commands/rpm_-qa_--qf_name_NAME_epoch_EPOCH_version_VERSION_release_RELEASE_arch_ARCH_installtime_INSTALLTIME_date_buildtime_BUILDTIME_vendor_VENDOR_buildhost_BUILDHOST_sigpgp_SIGPGP_pgpsig_n"
	machineIdFileName := "data/etc/insights-client/machine-id"

	systemComponentList := SystemComponents{}

	var buff *bytes.Buffer
	var found bool

	if buff, found = files[machineIdFileName]; !found {
		return fmt.Errorf("Failed to find machine ID")
	}

	systemComponentList.SystemID = string(buff.Bytes())

	if buff, found = files[packageListFileName]; !found {
		return fmt.Errorf("Failed to find package list file")
	}

	scanner := bufio.NewScanner(bytes.NewReader(buff.Bytes()))

	for scanner.Scan() {
		var c Component
		if err := json.Unmarshal(scanner.Bytes(), &c); err != nil {
			return fmt.Errorf("Error unmarshaling component: %w", err)
		}
		systemComponentList.Components = append(systemComponentList.Components, c)
	}

	systemComponents = append(systemComponents, systemComponentList)

	return nil
}

func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	// Limit the size of the upload to avoid excessive memory usage
	r.ParseMultipartForm(10 << 20) // 10 MB limit

	// Get all files from the multipart form
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		http.Error(w, "No files uploaded", http.StatusBadRequest)
		return
	}

	filesInMemory := make(map[string]*bytes.Buffer)
	fileHeader := files[0]

	// Open the file
	file, err := fileHeader.Open()
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to open file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// gunzip
	reader, err := gzip.NewReader(file)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to decompress file: %v", err), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	if err := untar(reader, filesInMemory); err != nil {
		http.Error(w, fmt.Sprintf("Error while extracting tarball: %v", err), http.StatusInternalServerError)
		return
	}

	if err := extractPackages(filesInMemory); err != nil {
		http.Error(w, fmt.Sprintf("Failed to extract components: %v", err), http.StatusInternalServerError)
		return
	}

	// Respond with the file information
	fmt.Fprintf(w, "File %s uploaded successfully!\n", fileHeader.Filename)
}

func handleList(w http.ResponseWriter, r *http.Request) {
	bytes, err := json.Marshal(SystemComponentList{Systems: systemComponents})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to marshal JSON response: %v", err), http.StatusInternalServerError)
		return
	}

	w.Write(bytes)
}

func main() {
	http.HandleFunc("/upload", handleFileUpload)
	http.HandleFunc("/upload/{machineid}", handleFileUpload)
	http.HandleFunc("/list", handleList)

	fmt.Println("Starting server on :8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		println(fmt.Errorf("Error starting server: %w", err))
	}
}
