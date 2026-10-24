package webhook

type Publisher interface{ Publish(repositoryID string) }
