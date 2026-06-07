package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"gitpier/internal/config"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitpier/internal/services"

	gossh "golang.org/x/crypto/ssh"
)

type Server struct {
	cfg         *config.Config
	authSvc     *services.AuthService
	repoSvc     *services.RepoService
	gitSvc      *services.GitService
	workflowSvc *services.WorkflowService
	webhookSvc  *services.WebhookService
}

func NewServer(cfg *config.Config, authSvc *services.AuthService, repoSvc *services.RepoService, gitSvc *services.GitService) *Server {
	return &Server{
		cfg:     cfg,
		authSvc: authSvc,
		repoSvc: repoSvc,
		gitSvc:  gitSvc,
	}
}

func (s *Server) SetWorkflowService(svc *services.WorkflowService) {
	s.workflowSvc = svc
}

func (s *Server) SetWebhookService(svc *services.WebhookService) {
	s.webhookSvc = svc
}

func (s *Server) ListenAndServe(addr string) error {
	hostKey, err := loadOrGenerateHostKey(s.cfg.SSHHostKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load SSH host key: %w", err)
	}

	sshConfig := &gossh.ServerConfig{
		PublicKeyCallback: s.publicKeyCallback,
	}
	sshConfig.AddHostKey(hostKey)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	defer listener.Close()

	log.Printf("SSH server listening on %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("SSH accept error: %v", err)
			continue
		}
		go s.handleConn(conn, sshConfig)
	}
}

func (s *Server) publicKeyCallback(conn gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
	fingerprint := gossh.FingerprintSHA256(key)

	user, err := s.authSvc.GetUserBySSHFingerprint(context.Background(), fingerprint)
	if err != nil {
		return nil, fmt.Errorf("unauthorized: key not found")
	}

	return &gossh.Permissions{
		Extensions: map[string]string{
			"user_id":  user.ID,
			"username": user.Username,
		},
	}, nil
}

func (s *Server) handleConn(conn net.Conn, cfg *gossh.ServerConfig) {
	defer conn.Close()

	sshConn, chans, reqs, err := gossh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sshConn.Close()

	go gossh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(gossh.UnknownChannelType, "unknown channel type")
			continue
		}
		ch, requests, err := newChan.Accept()
		if err != nil {
			return
		}
		go s.handleSession(ch, requests, sshConn.Permissions)
	}
}

func (s *Server) handleSession(ch gossh.Channel, reqs <-chan *gossh.Request, perms *gossh.Permissions) {
	defer ch.Close()

	for req := range reqs {
		switch req.Type {
		case "exec":
			if req.WantReply {
				req.Reply(true, nil)
			}
			s.handleExec(ch, req.Payload, perms)
			return
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

func (s *Server) handleExec(ch gossh.Channel, payload []byte, perms *gossh.Permissions) {
	if len(payload) < 4 {
		writeExitStatus(ch, 1)
		return
	}

	cmdLen := binary.BigEndian.Uint32(payload[:4])
	// Validate length without overflow: ensure payload[4:] is at least cmdLen bytes.
	if uint64(len(payload)) < 4+uint64(cmdLen) {
		writeExitStatus(ch, 1)
		return
	}
	cmdStr := string(payload[4 : 4+cmdLen])

	// Parse: git-receive-pack '/owner/repo.git' or git-upload-pack '/owner/repo.git'
	parts := strings.Fields(cmdStr)
	if len(parts) < 2 {
		fmt.Fprintf(ch.Stderr(), "error: invalid command\n")
		writeExitStatus(ch, 1)
		return
	}

	gitCmd := parts[0]
	if gitCmd != "git-receive-pack" && gitCmd != "git-upload-pack" {
		fmt.Fprintf(ch.Stderr(), "error: unsupported command %q\n", gitCmd)
		writeExitStatus(ch, 1)
		return
	}

	// Trim quotes and leading slash
	repoArg := strings.Trim(parts[1], "'\"")
	repoArg = strings.TrimPrefix(repoArg, "/")

	segments := strings.SplitN(repoArg, "/", 2)
	if len(segments) != 2 {
		fmt.Fprintf(ch.Stderr(), "error: invalid repository path\n")
		writeExitStatus(ch, 1)
		return
	}

	ownerUsername := segments[0]
	repoName := strings.TrimSuffix(segments[1], ".git")

	// Resolve repo
	repo, err := s.repoSvc.GetByOwnerAndName(context.Background(), ownerUsername, repoName)
	if err != nil {
		fmt.Fprintf(ch.Stderr(), "error: repository not found\n")
		writeExitStatus(ch, 1)
		return
	}

	// Resolve user from SSH permissions
	userID := perms.Extensions["user_id"]

	isWrite := gitCmd == "git-receive-pack"
	if isWrite && repo.IsArchived {
		fmt.Fprintf(ch.Stderr(), "error: repository is archived and read-only\n")
		writeExitStatus(ch, 1)
		return
	}

	if !s.repoSvc.HasAccess(repo, userID, isWrite) {
		fmt.Fprintf(ch.Stderr(), "error: access denied\n")
		writeExitStatus(ch, 1)
		return
	}

	// Execute git command
	repoPath := s.repoSvc.RepoPath(ownerUsername, repoName)

	// Check size limits before accepting push
	if isWrite {
		if sizeStatus, err := s.repoSvc.CheckSizeLimit(repo, repoPath); err != nil {
			fmt.Fprintf(ch.Stderr(), "error: %v\n", err)
			fmt.Fprintf(ch.Stderr(), "remote: %s\n", sizeStatus)
			writeExitStatus(ch, 1)
			return
		} else if sizeStatus != "" {
			// Show size status/warning (non-fatal)
			fmt.Fprintf(ch.Stderr(), "remote: %s\n", sizeStatus)
		}
	}

	// Snapshot refs before push so we can detect what changed.
	var oldRefs map[string]string
	if isWrite && (s.workflowSvc != nil || s.webhookSvc != nil) {
		oldRefs, _ = s.gitSvc.GetAllRefs(repoPath)
	}

	cmd := exec.Command(gitCmd, repoPath)
	cmd.Stdin = ch
	cmd.Stdout = ch
	cmd.Stderr = ch.Stderr()

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	// After a successful push, trigger matching workflows and fire webhooks asynchronously.
	if isWrite && exitCode == 0 {
		capturedRepo := repo
		capturedOwner := ownerUsername
		capturedRepoName := repoName
		capturedOldRefs := oldRefs
		capturedPusherName := perms.Extensions["username"]
		capturedPusherID := perms.Extensions["user_id"]
		capturedBaseURL := s.cfg.AppURL
		capturedRepoPath := repoPath
		go func() {
			// Update top language after push
			if lang := s.gitSvc.GetTopLanguage(capturedRepoPath, capturedRepo.DefaultBranch); lang != "" {
				_ = s.repoSvc.UpdateLanguage(context.Background(), capturedRepo.ID, lang)
			}

			// Update repository size after push
			_ = s.repoSvc.UpdateRepoSize(context.Background(), capturedRepo, capturedRepoPath)

			services.HandleSuccessfulPush(
				context.Background(),
				s.gitSvc,
				s.repoSvc,
				s.workflowSvc,
				s.webhookSvc,
				capturedRepo,
				capturedOwner,
				capturedRepoName,
				capturedRepoPath,
				capturedBaseURL,
				capturedPusherName,
				"",
				capturedPusherID,
				capturedOldRefs,
			)
		}()
	}

	writeExitStatus(ch, uint32(exitCode))
}

func writeExitStatus(ch gossh.Channel, code uint32) {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, code)
	ch.SendRequest("exit-status", false, payload)
}

func loadOrGenerateHostKey(path string) (gossh.Signer, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := generateAndSaveKey(path); err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read host key: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from host key")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host key: %w", err)
	}

	signer, err := gossh.NewSignerFromKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	return signer, nil
}

func generateAndSaveKey(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return err
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	return os.WriteFile(path, privPEM, 0600)
}

// Stderr returns a writer that sends data as SSH stderr (extended data type 1).
// gossh.Channel already implements Stderr() io.ReadWriter so this is handled by the library.
// We just need to ensure ch.Stderr() is used properly in cmd.Stderr.
var _ io.Writer = (gossh.Channel)(nil)
