import { Link } from "react-router-dom";

interface AppLogoProps {
  linkToHome?: boolean;
}

export function AppLogo({ linkToHome = true }: AppLogoProps) {
  const img = <img src="/assets/icon-128.png" alt="Trinity" className="app-logo" />;

  if (linkToHome) {
    return <Link to="/">{img}</Link>;
  }

  return img;
}
