import { useEffect, useState } from "react";
import { BarLoader } from "react-spinners";

const Loader = () => {
  const [isDarkMode, setIsDarkMode] = useState(false);

  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: dark)');
    const handleChange = () => setIsDarkMode(media.matches);

    // Initial check
    setIsDarkMode(media.matches);

    // Listen for changes
    media.addEventListener('change', handleChange);

    // Cleanup
    return () => media.removeEventListener('change', handleChange);
  }, []);

  const override = {
    margin: "0 auto",
    color: isDarkMode ? "white" : "black",
  }

  return (
    <BarLoader color={override.color} />
  )

}

export default Loader;
