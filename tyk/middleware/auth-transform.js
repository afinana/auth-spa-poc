var authTransformMiddleware = new TykJS.TykMiddleware.NewMiddleware({});

authTransformMiddleware.NewProcessRequest(function(request, session) {
  // 1. Mandatory zero-trust APIM boundary security headers
  request.SetHeaders["X-Gateway-Token"] = "project-poc-gateway-secret-token";
  request.SetHeaders["X-Enforcement-Point"] = "Tyk-APIM-Boundary";

  // 2. Extract and decode Bearer JWT to populate downstream identity headers
  var authHeader = request.Headers["Authorization"] || request.Headers["authorization"];
  var authStr = "";
  if (Array.isArray(authHeader) && authHeader.length > 0) {
    authStr = authHeader[0];
  } else if (typeof authHeader === "string") {
    authStr = authHeader;
  }

  if (authStr) {
    var token = authStr.replace(/^Bearer\s+/i, "").trim();
    if (token) {
      try {
        var parts = token.split(".");
        if (parts.length >= 2) {
          var base64Url = parts[1];
          var base64 = base64Url.replace(/-/g, "+").replace(/_/g, "/");
          while (base64.length % 4) {
            base64 += "=";
          }
          var jsonPayload = b64dec(base64);
          var claims = JSON.parse(jsonPayload);

          var username = claims.preferred_username || claims.sub || "unknown";
          var email = claims.email || "";

          var roles = [];
          if (claims.roles) {
            roles = claims.roles;
          } else if (claims.realm_access && claims.realm_access.roles) {
            roles = claims.realm_access.roles;
          }

          var roleStr = Array.isArray(roles) ? roles.join(",") : String(roles || "");

          request.SetHeaders["X-User-Username"] = username;
          request.SetHeaders["X-User-Email"] = email;
          request.SetHeaders["X-User-Role"] = roleStr;
        }
      } catch (err) {
        log("[Project Auth Transform] Error decoding JWT claims: " + err);
      }
    }
  }

  // 3. Clear upstream Authorization header to prevent token leakage
  if (request.DeleteHeaders) {
    request.DeleteHeaders.push("Authorization");
    request.DeleteHeaders.push("authorization");
  }

  return authTransformMiddleware.ReturnData(request, session || {});
});
