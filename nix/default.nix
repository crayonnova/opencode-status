{ buildGoModule, lib }:
buildGoModule {
  pname = "opencode-status";
  version = "0.1.0";
  src = ./..;
  # vendorHash: replace with the real SRI hash from the first failed build attempt.
  vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
  modRoot = ".";
  subPackages = [ "cmd/opencode-status" ];
  ldflags = [ "-s" "-w" ];
  meta = with lib; {
    description = "Monitor opencode free models: TUI + HTTP API + SQLite history";
    license = licenses.mit;
    mainProgram = "opencode-status";
    platforms = [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" ];
  };
}
