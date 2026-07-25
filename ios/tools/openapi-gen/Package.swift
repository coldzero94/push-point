// swift-tools-version:5.9
// swift-openapi-generator를 **CLI로** 실행하기 위한 최소 패키지.
// SPM 빌드 플러그인을 쓰지 않는 이유는 .claude/rules/api.md의 결정(재현성·드리프트 검사)이다 —
// 생성물은 커밋하고, 이 패키지는 그 생성기를 가져오는 용도로만 존재한다.
import PackageDescription

let package = Package(
    name: "openapi-gen",
    dependencies: [
        .package(url: "https://github.com/apple/swift-openapi-generator", from: "1.0.0"),
    ]
)
