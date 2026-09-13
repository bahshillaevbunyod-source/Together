import Image from "next/image";
import { X } from "lucide-react";

type Person = {
  name: string;
  location: string;
  interests: string;
  avatar: string;
};

const people: Person[] = [
  {
    name: "Timur Yusupov",
    location: "Samarkand, Uzbekistan",
    interests: "History, Architecture, Tea",
    avatar: "/images/avatars/timur-yusupov.webp",
  },
  {
    name: "Yuki Tanaka",
    location: "Tokyo, Japan",
    interests: "Design, Anime, Coffee",
    avatar: "/images/avatars/yuki-tanaka.webp",
  },
  {
    name: "David Okafor",
    location: "Lagos, Nigeria",
    interests: "Music, Tech, Football",
    avatar: "/images/avatars/david-okafor.webp",
  },
  {
    name: "Priya Sharma",
    location: "Mumbai, India",
    interests: "Art, Culture, Yoga",
    avatar: "/images/avatars/priya-sharma.webp",
  },
  {
    name: "Camila Souza",
    location: "São Paulo, Brazil",
    interests: "Travel, Dance, Food",
    avatar: "/images/avatars/camila-souza.webp",
  },
];

export function SuggestedPeople() {
  return (
    <section className="mr-1 rounded-2xl border border-border bg-surface p-5 shadow-sm">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-foreground">Suggested for you</h2>
        <button
          type="button"
          className="text-xs font-medium text-primary outline-none transition-colors hover:text-primary-hover focus:outline-none focus-visible:outline-none"
        >
          See all
        </button>
      </div>

      <ul className="mt-4 flex flex-col gap-3">
        {people.slice(0, 5).map((person) => (
          <li key={person.name} className="flex items-center gap-3">
            <Image
              src={person.avatar}
              alt={person.name}
              width={48}
              height={48}
              className="h-12 w-12 shrink-0 rounded-full object-cover"
            />
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-semibold text-foreground">
                {person.name}
              </div>
              <div className="truncate text-xs text-muted">{person.location}</div>
              <div className="truncate text-xs text-muted-soft">
                {person.interests}
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-1.5">
              <button
                type="button"
                className="rounded-full bg-primary-soft px-3 py-1 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
              >
                Follow
              </button>
              <button
                type="button"
                aria-label={`Dismiss ${person.name}`}
                className="flex h-6 w-6 items-center justify-center rounded-full text-muted-soft transition-colors hover:bg-background hover:text-muted"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
