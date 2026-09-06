{ pkgs, lib, ... }:
let
  casambiClient = pkgs.python3Packages.buildPythonPackage {
    pname = "casambi-bt";
    version = "0.3.2";
    pyproject = true;
    src = pkgs.fetchFromGitHub {
      owner = "lkempf";
      repo = "casambi-bt";
      rev = "ec23769ab3459abf8ba5f332267900964319d03e";
      hash = "sha256-nQY47bkuCnpJMu7VCEtRiLLTnkrrhmiGI2ysVtWfivk=";
    };
    build-system = [ pkgs.python3Packages.setuptools ];
    dependencies = with pkgs.python3Packages; [
      bleak
      bleak-retry-connector
      cryptography
      httpx
      anyio
    ];
    pythonImportsCheck = [ "CasambiBt" ];
  };
in
{
  packages = [
    (pkgs.python3.withPackages (_: [ casambiClient ]))
  ] ++ lib.optionals pkgs.stdenv.isLinux [ pkgs.bluez pkgs.dbus ];
}
