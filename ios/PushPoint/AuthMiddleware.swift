import Foundation
import HTTPTypes
import OpenAPIRuntime

/// Bearer 인증을 모든 요청에 붙이는 미들웨어.
///
/// 손으로 쓰는 이유는 swift-openapi-generator가 **securityScheme에 대한 클라이언트 코드를
/// 생성하지 않기 때문**이다(.claude/rules/api.md에 기록된 기지 제약). 요청·응답 타입은
/// 여전히 전부 생성물이고, 여기서 더하는 것은 헤더 한 줄뿐이다.
struct AuthMiddleware: ClientMiddleware {
    let apiKey: String

    func intercept(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String,
        next: (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        var request = request
        request.headerFields[.authorization] = "Bearer \(apiKey)"
        return try await next(request, body, baseURL)
    }
}
