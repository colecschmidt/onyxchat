package com.cole.securechat.ui;

import javafx.fxml.FXMLLoader;
import javafx.scene.Parent;
import javafx.scene.Scene;
import javafx.stage.Stage;

import com.cole.securechat.util.AppConfig;
import javafx.application.Platform;
import javafx.fxml.FXML;
import javafx.scene.control.Alert;
import javafx.scene.control.Button;
import javafx.scene.control.PasswordField;
import javafx.scene.control.TextField;
import javafx.scene.control.TextInputDialog;

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.concurrent.FutureTask;

import org.json.JSONObject;

public class LoginController {

    @FXML
    private TextField usernameField;

    @FXML
    private PasswordField passwordField;

    @FXML
    private Button loginButton;

    // Built-in HTTP client (Java 11+)
    private final HttpClient httpClient = HttpClient.newHttpClient();

    @FXML
    private void initialize() {
        // Any setup logic later
    }

    @FXML
    private void handleLoginButton() {
        String username = usernameField.getText().trim();
        String password = passwordField.getText().trim();

        if (username.isEmpty() || password.isEmpty()) {
            showError("Username and password are required.");
            return;
        }

        loginButton.setDisable(true);

        new Thread(() -> {
            boolean success = false;
            String errorMessage = null;

            long userIdFromServer = -1;
            String usernameFromServer = null;
            String tokenFromServer = null;

            try {
                JSONObject obj = postJson("/api/v1/login", String.format(
                        "{\"username\":\"%s\",\"password\":\"%s\"}",
                        escapeJson(username),
                        escapeJson(password)
                ));

                // An account with a second factor gets a challenge here instead
                // of a session: the password is accepted, but no token is issued
                // until a code is verified at /api/v1/login/2fa.
                if (obj.optBoolean("mfa_required", false)) {
                    String mfaToken = obj.getString("mfa_token");
                    String code = promptForVerificationCode();

                    if (code == null || code.isEmpty()) {
                        throw new IOException("Sign-in cancelled — a verification code is required.");
                    }

                    obj = postJson("/api/v1/login/2fa", String.format(
                            "{\"mfa_token\":\"%s\",\"code\":\"%s\"}",
                            escapeJson(mfaToken),
                            escapeJson(code)
                    ));
                }

                userIdFromServer = obj.getLong("id");
                usernameFromServer = obj.getString("username");
                tokenFromServer = obj.getString("token");

                if (userIdFromServer <= 0 ||
                        usernameFromServer == null || usernameFromServer.isEmpty() ||
                        tokenFromServer == null || tokenFromServer.isEmpty()) {

                    errorMessage = "Could not parse login response: " + obj;
                } else {
                    success = true;
                }

            } catch (Exception e) {
                e.printStackTrace();
                errorMessage = e.getMessage();
            }

            boolean finalSuccess = success;
            String finalErrorMessage = errorMessage;
            long finalUserId = userIdFromServer;
            String finalUsername = (usernameFromServer != null) ? usernameFromServer : username;
            String finalToken = tokenFromServer;

            Platform.runLater(() -> {
                loginButton.setDisable(false);

                if (finalSuccess) {
                    // ✅ store the JWT for later HTTP/WebSocket calls
                    AppConfig.AUTH_TOKEN = finalToken;

                    showInfo("Login successful!");

                    try {
                        Stage stage = (Stage) loginButton.getScene().getWindow();
                        FXMLLoader loader = new FXMLLoader(getClass().getResource("/chat_window.fxml"));
                        Parent chatRoot = loader.load();

                        ChatWindowController controller = loader.getController();
                        // use the final* variables defined above
                        controller.setCurrentUser(finalUserId, finalUsername, finalToken);

                        // Reuse the existing scene from Main (keeps onyx-dark.css)
                        Scene scene = stage.getScene();
                        scene.setRoot(chatRoot);
                        stage.setTitle("Onyx Chat");
                        stage.show();

                    } catch (Exception e) {
                        e.printStackTrace();
                        if (e.getCause() != null) {
                            e.getCause().printStackTrace();
                        }
                        showError("Failed to load chat window: " + e.getMessage());
                    }
                } else {
                    showError(finalErrorMessage != null ? finalErrorMessage : "Login failed.");
                }
            });
        }, "login-thread").start();
    }


    /**
     * POSTs a JSON body and returns the parsed response, throwing on any
     * non-2xx status so the caller has a single failure path.
     */
    private JSONObject postJson(String path, String jsonBody) throws Exception {
        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(AppConfig.SERVER_BASE_URL + path))
                .header("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(jsonBody))
                .build();

        HttpResponse<String> response =
                httpClient.send(request, HttpResponse.BodyHandlers.ofString());

        int status = response.statusCode();
        String body = response.body();
        System.out.println("POST " + path + " -> " + status);

        if (status != 200 && status != 201) {
            throw new IOException("Server returned " + status + ": " + body);
        }
        return new JSONObject(body);
    }

    /**
     * Asks for the second factor. Dialogs must be built on the JavaFX
     * application thread, but the login runs on a worker thread, so the prompt
     * is handed over as a FutureTask the worker blocks on.
     *
     * @return the trimmed code, or null if the user dismissed the dialog
     */
    private String promptForVerificationCode() throws Exception {
        FutureTask<String> prompt = new FutureTask<>(() -> {
            TextInputDialog dialog = new TextInputDialog();
            dialog.setTitle("Two-factor verification");
            dialog.setHeaderText("Enter the 6-digit code from your authenticator app.");
            dialog.setContentText("Code (or a recovery code):");
            return dialog.showAndWait().map(String::trim).orElse(null);
        });
        Platform.runLater(prompt);
        return prompt.get();
    }

    private String escapeJson(String s) {
        return s.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    private void showError(String msg) {
        Alert alert = new Alert(Alert.AlertType.ERROR);
        alert.setTitle("Login failed");
        alert.setHeaderText(null);
        alert.setContentText(msg);
        alert.showAndWait();
    }

    private void showInfo(String msg) {
        Alert alert = new Alert(Alert.AlertType.INFORMATION);
        alert.setTitle("Login");
        alert.setHeaderText(null);
        alert.setContentText(msg);
        alert.showAndWait();
    }
}
