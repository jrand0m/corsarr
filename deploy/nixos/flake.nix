{
  description = "corsarr media automation server: release binaries and a NixOS service";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.05";

  outputs = { self, nixpkgs }:
    let
      inherit (nixpkgs) lib;
      version = "0.4.0";
      base = "https://github.com/jrand0m/corsarr/releases/download/v${version}";
      # release archives and their SHA256SUMS entries (just release pins updates both)
      archives = {
        x86_64-linux = {
          url = "${base}/corsarr-linux-amd64.tar.gz";
          sha256 = "e8063b0ed93fcec330a220ff5bb0e60963a4af419a3be0098d262b3c4b14169a";
        };
        aarch64-linux = {
          url = "${base}/corsarr-linux-arm64.tar.gz";
          sha256 = "83a5bd2545a401408293a7e4865f7f4a114911c2329ab6f38b8e0ede3fafb5c1";
        };
        x86_64-darwin = {
          url = "${base}/corsarr-darwin-amd64.tar.gz";
          sha256 = "7e7ba9c15bea1645bdff4f598b1ba5280905a3388347d5774acc945bc8ca49c8";
        };
        aarch64-darwin = {
          url = "${base}/corsarr-darwin-arm64.tar.gz";
          sha256 = "29c84c51d9347f35c9a46529adf8870125431c43e734d62f53588af23e5d1df0";
        };
      };
      systems = builtins.attrNames archives;
      linuxSystems = lib.filter (lib.hasSuffix "-linux") systems;
      forSystems = sys: f: lib.genAttrs sys (system: f nixpkgs.legacyPackages.${system});

      # the static release binary, unchanged
      mkCorsarr = pkgs:
        pkgs.stdenvNoCC.mkDerivation {
          pname = "corsarr";
          inherit version;
          src = pkgs.fetchurl archives.${pkgs.stdenv.hostPlatform.system};
          sourceRoot = ".";
          dontConfigure = true;
          dontBuild = true;
          dontStrip = true;
          installPhase = ''
            runHook preInstall
            install -Dm0755 corsarr-*/corsarr $out/bin/corsarr
            install -Dm0644 corsarr-*/LICENSE $out/share/licenses/corsarr/LICENSE
            runHook postInstall
          '';
          doInstallCheck = true;
          installCheckPhase = "$out/bin/corsarr version";
          meta = {
            description = "Media automation server in one static binary";
            homepage = "https://github.com/jrand0m/corsarr";
            license = lib.licenses.agpl3Plus;
            sourceProvenance = [ lib.sourceTypes.binaryNativeCode ];
            platforms = systems;
            mainProgram = "corsarr";
          };
        };

      # a minimal system using the module, for the checks
      exampleSystem = system:
        lib.nixosSystem {
          inherit system;
          modules = [
            self.nixosModules.default
            {
              boot.isContainer = true;
              system.stateVersion = "25.05";
              services.corsarr = {
                enable = true;
                openFirewall = true;
                mediaPaths = [ "/srv/media" ];
              };
            }
          ];
        };
    in
    {
      packages = forSystems systems (pkgs: rec {
        corsarr = mkCorsarr pkgs;
        default = corsarr;
      });

      overlays.default = final: _prev: { corsarr = mkCorsarr final; };

      nixosModules.corsarr = import ./module.nix self;
      nixosModules.default = self.nixosModules.corsarr;

      checks = forSystems linuxSystems (pkgs:
        let
          sys = exampleSystem pkgs.stdenv.hostPlatform.system;
          etc = sys.config.environment.etc."corsarr/corsarr.yml".source;
        in
        {
          package = self.packages.${pkgs.stdenv.hostPlatform.system}.corsarr;
          unit = sys.config.systemd.units."corsarr.service".unit;
          # the generated corsarr.yml passes corsarr's own validation
          config = pkgs.runCommand "corsarr-config-check" { } ''
            ${lib.getExe self.packages.${pkgs.stdenv.hostPlatform.system}.corsarr} config validate --configFile=${etc}
            touch $out
          '';
        });
    };
}
