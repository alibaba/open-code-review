import React, { useRef, useEffect, useState } from 'react';

interface FadeInSectionProps {
  children: React.ReactNode;
  delay?: number; // ms
}

const FadeInSection: React.FC<FadeInSectionProps> = ({ children, delay = 0 }) => {
  const ref = useRef<HTMLDivElement>(null);
  const [isVisible, setIsVisible] = useState(false);

  useEffect(() => {
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setIsVisible(true);
          observer.unobserve(entry.target);
        }
      },
      { threshold: 0.08 }
    );

    if (ref.current) {
      observer.observe(ref.current);
    }

    return () => observer.disconnect();
  }, []);

  return (
    <div
      ref={ref}
      style={{
        opacity: isVisible ? 1 : 0,
        transition: `opacity 0.6s ease ${delay}ms`,
      }}
    >
      {children}
    </div>
  );
};

export default FadeInSection;
