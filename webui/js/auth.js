const Auth = (function() {

  const config = {
    issuer: 'https://auth.nextiva.xyz/oauth2/austgyztgfRpJ4Qr40h7',
    redirectUri: 'http://localhost:8000/webui/',
    clientId: '0oawrrlkmf5pYvHYD0h7',
    scopes: 'openid email',
    storage: 'sessionStorage',
    requireUserSession: 'true',
    flow: 'redirect'
  };

  const authClient = new OktaAuth({
    issuer: config.issuer,
    clientId: config.clientId,
    redirectUri: config.redirectUri,
    scopes: config.scopes.split(/\s+/),
    tokenManager: {
      storage: config.storage
    },
    //transformAuthState
  });

  authClient.authStateManager.subscribe(function(authState) {
    if (!authState.isAuthenticated) {
      // If not authenticated, reset values related to user session
      userInfo = null;
    }

    // If there is an active session, we can get tokens via a redirect
    // This allows in-memory token storage without prompting for credentials on each page load
    if (shouldRedirectToGetTokens(authState)) {
      return redirectToGetTokens();
    }

    // Render app based on the new authState
    renderApp();
  });

  function shouldRedirectToGetTokens(authState) {
    if (authState.isAuthenticated || authState.isPending) {
      return false;
    }
  
    // Special handling for memory-based token storage.
    // There will be a redirect on each page load to acquire fresh tokens.
    if (config.storage === 'memory' || config.getTokens) {
  
      // Callback from Okta triggered by `redirectToGetTokens`
      // If the callback has errored, it means there is no Okta session and we should begin a new auth flow
      // This condition breaks a potential infinite rediret loop
      if (config.error === 'login_required') {
        return false;
      }
  
      // Call Okta to get tokens. Okta will redirect back to this app
      // The callback is handled by `handleLoginRedirect` which will call `renderApp` again
      return true;
    }
  }

  function redirectToGetTokens(additionalParams) {
    // If an Okta SSO exists, the redirect will return a code which can be exchanged for tokens
    // If a session does not exist, it will return with "error=login_required"

    redirectToLogin(Object.assign({
      prompt: 'none',
    }, additionalParams));
  }

  function redirectToLogin(additionalParams) {
    // Redirect to Okta and show the signin widget if there is no active session
    authClient.token.getWithRedirect(Object.assign({
      state: JSON.stringify(config.state),
    }, additionalParams));
  }
  
  function handleLoginRedirect() {
    // The URL contains a code, `parseFromUrl` will exchange the code for tokens
    return authClient.token.parseFromUrl().then(function (res) {
      endAuthFlow(res.tokens); // save tokens
    }).catch(function(error) {
      console.error(error);
    });
  }

  function endAuthFlow(tokens) {
    // parseFromUrl clears location.search. There may also be a leftover "error" param from the auth flow.
    // Replace state with the canonical app uri so the page can be reloaded cleanly.
    history.replaceState(null, '', config.appUri);
  
    // Store tokens. This will update the auth state and we will re-render
    authClient.tokenManager.setTokens(tokens);
  }

  function initialize() {
    // Calculate initial auth state and fire change event for listeners
    authClient.authStateManager.updateAuthState();
  }

  function renderApp() {
    const authState = authClient.authStateManager.getAuthState();

    // If auth state is "pending", render in the loading state
    if (authState.isPending) {
      console.log('Loading...');
      return;
    }

    // Not loading
    if (authState.isAuthenticated) {
      console.log('Logged In...');
      App();
      return;
    }

    // Default: Unauthenticated state
    redirectToLogin();
    return;
  }

  function run() {
    // During the OIDC auth flow, the app will receive a code passed to the `redirectUri`
    // This event occurs *in the middle* of an authorization flow
    // The callback handler logic should happen *before and instead of* any other auth logic
    // In most apps this callback will be handled by a special route
    // For SPA apps like this, with no routing or hash-based routing, the callback is handled in the main function
    // Once the callback is handled, the app can startup normally
    if (authClient.token.isLoginRedirect()) {
      return handleLoginRedirect().then(function() {
        initialize();
      });
    }

    initialize();
  }

  run();
  
  return {
    access_token: function() {
      return authClient.getAccessToken();
    }
  }
})();
